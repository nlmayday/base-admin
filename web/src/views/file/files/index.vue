<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from "vue";
import { useDebounceFn } from "@vueuse/core";
import dayjs from "dayjs";
import { message } from "@/utils/message";
import { hasPerm } from "@/utils/permission";
import { usePagedTable } from "@/composables/usePagedTable";
import { confirmAction } from "@/composables/useConfirmAction";
import {
  deleteFile,
  getUploadPolicy,
  listCategories,
  listFiles,
  type CategoryList,
  type FileItem,
  type FileKind,
  type UploadPolicy
} from "@/api/file";
import CategoryPanel from "./components/CategoryPanel.vue";
import FileGrid from "./components/FileGrid.vue";
import FileTable from "./components/FileTable.vue";
import UploadDialog from "./components/UploadDialog.vue";
import PreviewDialog from "./components/PreviewDialog.vue";
import FileEditDialog, { type EditMode } from "./components/FileEditDialog.vue";
import type { FileAction, ViewMode } from "./types";
import {
  KIND_OPTIONS,
  absoluteUrl,
  copyText,
  downloadFile,
  forgetSignedUrl,
  signedUrlFor
} from "./utils";
import SearchIcon from "~icons/ri/search-line";
import GridIcon from "~icons/ri/layout-grid-line";
import ListIcon from "~icons/ri/list-unordered";
import UploadIcon from "~icons/ri/upload-2-line";
import UploadCloudIcon from "~icons/ri/upload-cloud-2-line";

defineOptions({ name: "FileLibrary" });

const VIEW_STORAGE_KEY = "file-library:view";
const PAGE_SIZES = [24, 48, 96];

const canUpload = computed(() => hasPerm("file:file:upload"));

const filter = reactive<{
  keyword: string;
  categoryId: string;
  kind: FileKind | "";
}>({ keyword: "", categoryId: "", kind: "" });

const {
  loading,
  rows,
  pagination,
  reload,
  search,
  onSizeChange,
  onCurrentChange
} = usePagedTable<FileItem>(query => listFiles({ ...query, ...filter }));
pagination.pageSize = PAGE_SIZES[0];

const categories = ref<CategoryList | null>(null);
const categoriesLoading = ref(false);
const policy = ref<UploadPolicy | null>(null);

async function loadCategories(): Promise<void> {
  categoriesLoading.value = true;
  try {
    categories.value = await listCategories();
  } catch {
    // 错误提示已由 http 拦截器统一处理
  } finally {
    categoriesLoading.value = false;
  }
}

async function loadPolicy(): Promise<void> {
  if (!canUpload.value) return;
  try {
    policy.value = await getUploadPolicy();
  } catch {
    policy.value = null;
  }
}

function reloadAll(): void {
  reload();
  loadCategories();
}

/* ---------- 视图模式（按浏览器记住） ---------- */

function readViewMode(): ViewMode {
  try {
    return localStorage.getItem(VIEW_STORAGE_KEY) === "list" ? "list" : "grid";
  } catch {
    return "grid";
  }
}

const viewMode = ref<ViewMode>(readViewMode());
watch(viewMode, mode => {
  try {
    localStorage.setItem(VIEW_STORAGE_KEY, mode);
  } catch {
    // 隐私模式等场景无法写入时忽略
  }
});

/* ---------- 筛选 ---------- */

const debouncedSearch = useDebounceFn(search, 300);
watch(() => filter.keyword, debouncedSearch);
watch(() => [filter.categoryId, filter.kind], search);

const currentCategoryName = computed(() => {
  if (filter.categoryId === "") return "全部文件";
  if (filter.categoryId === "none") return "未分类";
  return (
    categories.value?.list.find(c => c.id === filter.categoryId)?.name ?? "分类"
  );
});

const categoryOptions = computed(() => categories.value?.list ?? []);

/* ---------- 上传 ---------- */

const uploadVisible = ref(false);
const uploadDialog = ref<InstanceType<typeof UploadDialog>>();
const uploadTargetCategory = computed(() =>
  filter.categoryId && filter.categoryId !== "none" ? filter.categoryId : null
);
const refreshAfterUpload = useDebounceFn(reloadAll, 400);

async function openUpload(files?: File[]): Promise<void> {
  uploadVisible.value = true;
  // 等对话框按当前分类初始化目标分类后再加入队列
  await nextTick();
  if (files?.length) uploadDialog.value?.addFiles(files);
}

const dropActive = ref(false);
let dragDepth = 0;

function hasFiles(e: DragEvent): boolean {
  return Array.from(e.dataTransfer?.types ?? []).includes("Files");
}

function onDragEnter(e: DragEvent): void {
  if (!canUpload.value || uploadVisible.value || !hasFiles(e)) return;
  dragDepth++;
  dropActive.value = true;
}

function onDragLeave(): void {
  if (!dropActive.value) return;
  dragDepth = Math.max(0, dragDepth - 1);
  if (dragDepth === 0) dropActive.value = false;
}

function onDrop(e: DragEvent): void {
  if (!dropActive.value) return;
  dragDepth = 0;
  dropActive.value = false;
  openUpload(Array.from(e.dataTransfer?.files ?? []));
}

/* ---------- 文件操作 ---------- */

const previewVisible = ref(false);
const editVisible = ref(false);
const editMode = ref<EditMode>("rename");
const current = ref<FileItem | null>(null);

async function copyLink(file: FileItem): Promise<void> {
  try {
    const { url, expiresAt } = await signedUrlFor(file.id);
    await copyText(absoluteUrl(url));
    message(`链接已复制，${dayjs(expiresAt).format("HH:mm")} 前有效`, {
      type: "success"
    });
  } catch (err) {
    if (err instanceof Error && err.message === "copy failed") {
      message("复制失败，请检查浏览器剪贴板权限", { type: "error" });
    }
  }
}

async function download(file: FileItem): Promise<void> {
  try {
    await downloadFile(file);
  } catch {
    // 签名链接获取失败时拦截器已提示
  }
}

function remove(file: FileItem): void {
  confirmAction({
    confirm: `确定删除文件「${file.originalName}」吗？删除后无法恢复，已分享的链接也将失效。`,
    success: "已删除",
    action: async () => {
      await deleteFile(file.id);
      forgetSignedUrl(file.id);
      if (current.value?.id === file.id) previewVisible.value = false;
    },
    reload: () => {
      // 删除当前页最后一项时回到上一页
      if (rows.value.length === 1 && pagination.currentPage > 1) {
        pagination.currentPage--;
      }
      reloadAll();
    }
  });
}

function onAction(action: FileAction, file: FileItem): void {
  current.value = file;
  switch (action) {
    case "preview":
      previewVisible.value = true;
      return;
    case "copy-link":
      copyLink(file);
      return;
    case "download":
      download(file);
      return;
    case "rename":
    case "move":
      editMode.value = action;
      editVisible.value = true;
      return;
    case "delete":
      remove(file);
  }
}

function onSaved(saved: FileItem): void {
  const i = rows.value.findIndex(r => r.id === saved.id);
  if (i >= 0) {
    rows.value = rows.value.map(r => (r.id === saved.id ? saved : r));
  }
  if (editMode.value === "move") reloadAll();
}

function resetFilter(): void {
  filter.keyword = "";
  filter.kind = "";
}

onMounted(() => {
  reload();
  loadCategories();
  loadPolicy();
});
</script>

<template>
  <div class="main split-page flex gap-3">
    <div class="w-[240px] shrink-0">
      <CategoryPanel
        v-model="filter.categoryId"
        :data="categories"
        :loading="categoriesLoading"
        @changed="reloadAll"
      />
    </div>

    <div class="min-w-0 flex-1">
      <el-card shadow="never" class="mb-3" body-class="py-3!">
        <div class="flex flex-wrap items-center gap-3">
          <el-input
            v-model="filter.keyword"
            placeholder="搜索文件名"
            clearable
            class="w-[240px]!"
            data-testid="file-search"
          >
            <template #prefix><SearchIcon /></template>
          </el-input>
          <el-radio-group v-model="filter.kind">
            <el-radio-button value="">全部</el-radio-button>
            <el-radio-button
              v-for="o in KIND_OPTIONS"
              :key="o.value"
              :value="o.value"
            >
              {{ o.label }}
            </el-radio-button>
          </el-radio-group>
          <el-button
            v-if="filter.keyword || filter.kind"
            link
            type="primary"
            @click="resetFilter"
          >
            重置
          </el-button>
          <div class="ml-auto flex items-center gap-3">
            <el-radio-group v-model="viewMode" aria-label="视图">
              <el-radio-button value="grid" title="网格视图">
                <GridIcon />
              </el-radio-button>
              <el-radio-button value="list" title="列表视图">
                <ListIcon />
              </el-radio-button>
            </el-radio-group>
            <el-button
              v-perm="'file:file:upload'"
              type="primary"
              data-testid="file-upload"
              @click="openUpload()"
            >
              <UploadIcon class="mr-1" />上传文件
            </el-button>
          </div>
        </div>
      </el-card>

      <el-card
        shadow="never"
        class="relative"
        @dragenter.prevent="onDragEnter"
        @dragover.prevent
        @dragleave="onDragLeave"
        @drop.prevent="onDrop"
      >
        <div class="mb-3 flex items-center justify-between">
          <span class="font-medium">
            {{ currentCategoryName }}
            <span
              class="ml-1 text-xs font-normal text-[var(--el-text-color-secondary)]"
            >
              共 {{ pagination.total }} 个文件
            </span>
          </span>
          <span
            v-if="canUpload"
            class="hidden text-xs text-[var(--el-text-color-secondary)] md:inline"
          >
            可直接将文件拖入此区域上传
          </span>
        </div>

        <div v-loading="loading" class="min-h-[320px]">
          <template v-if="rows.length">
            <FileGrid
              v-if="viewMode === 'grid'"
              :files="rows"
              @action="onAction"
            />
            <FileTable v-else :files="rows" @action="onAction" />
          </template>
          <el-empty
            v-else-if="!loading"
            :description="
              filter.keyword || filter.kind
                ? '没有符合条件的文件'
                : '这里还没有文件'
            "
          >
            <el-button
              v-if="canUpload && !filter.keyword && !filter.kind"
              type="primary"
              @click="openUpload()"
            >
              <UploadIcon class="mr-1" />上传第一个文件
            </el-button>
          </el-empty>
        </div>

        <el-pagination
          v-if="pagination.total > 0"
          class="mt-4 justify-end"
          :current-page="pagination.currentPage"
          :page-size="pagination.pageSize"
          :page-sizes="PAGE_SIZES"
          :total="pagination.total"
          layout="total, sizes, prev, pager, next, jumper"
          background
          @size-change="onSizeChange"
          @current-change="onCurrentChange"
        />

        <transition name="el-fade-in">
          <div v-if="dropActive" class="drop-overlay">
            <UploadCloudIcon class="h-14 w-14" />
            <div class="mt-2 text-base font-medium">
              松开鼠标，上传到「{{
                uploadTargetCategory ? currentCategoryName : "未分类"
              }}」
            </div>
          </div>
        </transition>
      </el-card>
    </div>

    <UploadDialog
      ref="uploadDialog"
      v-model="uploadVisible"
      :categories="categoryOptions"
      :default-category-id="uploadTargetCategory"
      :policy="policy"
      @uploaded="refreshAfterUpload"
    />
    <PreviewDialog
      v-model="previewVisible"
      :file="current"
      @download="download"
      @copy-link="copyLink"
    />
    <FileEditDialog
      v-model="editVisible"
      :file="current"
      :mode="editMode"
      :categories="categoryOptions"
      @saved="onSaved"
      @reload="reloadAll"
    />
  </div>
</template>

<style scoped lang="scss">
.drop-overlay {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--el-color-primary);
  pointer-events: none;
  background: color-mix(in srgb, var(--el-color-primary) 8%, transparent);
  border: 2px dashed var(--el-color-primary);
  border-radius: 4px;
  backdrop-filter: blur(1px);
}
</style>
