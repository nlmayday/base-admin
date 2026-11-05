<script setup lang="ts">
import { computed, nextTick, ref } from "vue";
import type { InputInstance } from "element-plus";
import { message } from "@/utils/message";
import { ErrorCode, isApiError, needsReload } from "@/utils/http/errors";
import { handleApiError } from "@/utils/http/handler";
import { hasPerm } from "@/utils/permission";
import { confirmAction } from "@/composables/useConfirmAction";
import {
  createCategory,
  deleteCategory,
  updateCategory,
  type CategoryList,
  type FileCategory
} from "@/api/file";
import FoldersIcon from "~icons/ri/folders-line";
import FolderIcon from "~icons/ri/folder-3-line";
import FolderOpenIcon from "~icons/ri/folder-open-line";
import InboxIcon from "~icons/ri/inbox-line";
import AddIcon from "~icons/ri/add-line";
import EditIcon from "~icons/ri/edit-line";
import DeleteIcon from "~icons/ri/delete-bin-line";

defineOptions({ name: "FileCategoryPanel" });

const props = defineProps<{ data: CategoryList | null; loading: boolean }>();
/** "" = 全部，"none" = 未分类，其余为分类 id */
const selected = defineModel<string>({ required: true });
const emit = defineEmits<{ (e: "changed"): void }>();

const NAME_MAX = 64;

const canEdit = computed(() => hasPerm("file:file:update"));
const canDelete = computed(() => hasPerm("file:file:delete"));

const creating = ref(false);
const newName = ref("");
const editingId = ref<string | null>(null);
const editName = ref("");
const saving = ref(false);
const createInput = ref<InputInstance>();
const editInputs = ref<InputInstance[]>([]);

const categories = computed<FileCategory[]>(() => props.data?.list ?? []);

/** 行内编辑没有表单，字段错误（如重名）直接提示；其他错误交给全局处理 */
const SILENT = { skipErrorHandler: true } as const;

function reportError(err: unknown): void {
  if (!isApiError(err)) return;
  if (err.code === ErrorCode.VALIDATION && err.fields.length > 0) {
    message(err.fields[0].message, { type: "error" });
    return;
  }
  handleApiError(err, {});
}

function select(value: string): void {
  if (editingId.value) return;
  selected.value = value;
}

async function startCreate(): Promise<void> {
  editingId.value = null;
  creating.value = true;
  newName.value = "";
  await nextTick();
  createInput.value?.focus();
}

async function submitCreate(): Promise<void> {
  if (!creating.value || saving.value) return;
  const name = newName.value.trim();
  if (!name) {
    creating.value = false;
    return;
  }
  saving.value = true;
  try {
    const nextSort = categories.value.length
      ? Math.max(...categories.value.map(c => c.sort)) + 1
      : 0;
    const created = await createCategory(
      { name, sort: Math.min(nextSort, 9999) },
      SILENT
    );
    message("分类已创建", { type: "success" });
    creating.value = false;
    emit("changed");
    selected.value = created.id;
  } catch (err) {
    reportError(err);
  } finally {
    saving.value = false;
  }
}

async function startEdit(c: FileCategory): Promise<void> {
  creating.value = false;
  editingId.value = c.id;
  editName.value = c.name;
  await nextTick();
  editInputs.value[0]?.focus();
}

async function submitEdit(c: FileCategory): Promise<void> {
  if (editingId.value !== c.id || saving.value) return;
  const name = editName.value.trim();
  if (!name || name === c.name) {
    editingId.value = null;
    return;
  }
  saving.value = true;
  try {
    await updateCategory(
      c.id,
      { name, sort: c.sort, version: c.version },
      SILENT
    );
    message("分类已重命名", { type: "success" });
    editingId.value = null;
    emit("changed");
  } catch (err) {
    reportError(err);
    if (needsReload(err)) {
      editingId.value = null;
      emit("changed");
    }
  } finally {
    saving.value = false;
  }
}

function remove(c: FileCategory): void {
  confirmAction({
    confirm: `确定删除分类「${c.name}」吗？分类下有文件时不能删除。`,
    success: "分类已删除",
    action: () => deleteCategory(c.id),
    reload: () => {
      if (selected.value === c.id) selected.value = "";
      emit("changed");
    }
  });
}
</script>

<template>
  <el-card shadow="never" class="h-full" body-class="p-2!">
    <template #header>
      <div class="flex items-center justify-between">
        <span class="card-title">文件分类</span>
        <el-tooltip v-if="canEdit" content="新建分类" placement="top">
          <el-button
            link
            type="primary"
            data-testid="category-create"
            @click="startCreate"
          >
            <AddIcon class="text-lg" />
          </el-button>
        </el-tooltip>
      </div>
    </template>

    <div v-loading="loading" class="category-list">
      <div
        class="category-item"
        :class="{ 'is-active': selected === '' }"
        @click="select('')"
      >
        <FoldersIcon class="category-item__icon" />
        <span class="category-item__name">全部文件</span>
        <span class="category-item__count">{{ data?.total ?? 0 }}</span>
      </div>
      <div
        class="category-item"
        :class="{ 'is-active': selected === 'none' }"
        @click="select('none')"
      >
        <InboxIcon class="category-item__icon" />
        <span class="category-item__name">未分类</span>
        <span class="category-item__count">{{ data?.uncategorized ?? 0 }}</span>
      </div>

      <el-divider class="my-2!" />

      <div
        v-for="c in categories"
        :key="c.id"
        class="category-item group"
        :class="{ 'is-active': selected === c.id }"
        @click="select(c.id)"
      >
        <component
          :is="selected === c.id ? FolderOpenIcon : FolderIcon"
          class="category-item__icon"
        />
        <template v-if="editingId === c.id">
          <el-input
            ref="editInputs"
            v-model="editName"
            size="small"
            :maxlength="NAME_MAX"
            :disabled="saving"
            @click.stop
            @keyup.enter="submitEdit(c)"
            @keyup.esc="editingId = null"
            @blur="submitEdit(c)"
          />
        </template>
        <template v-else>
          <el-tooltip
            :content="c.name"
            placement="right"
            :show-after="600"
            :disabled="c.name.length < 10"
          >
            <span class="category-item__name">{{ c.name }}</span>
          </el-tooltip>
          <span class="category-item__count group-hover:hidden">
            {{ c.fileCount }}
          </span>
          <span
            v-if="canEdit || canDelete"
            class="category-item__actions hidden group-hover:flex"
          >
            <el-button
              v-if="canEdit"
              link
              size="small"
              title="重命名"
              @click.stop="startEdit(c)"
            >
              <EditIcon />
            </el-button>
            <el-button
              v-if="canDelete"
              link
              size="small"
              type="danger"
              title="删除"
              @click.stop="remove(c)"
            >
              <DeleteIcon />
            </el-button>
          </span>
        </template>
      </div>

      <div v-if="creating" class="category-item">
        <FolderIcon class="category-item__icon" />
        <el-input
          ref="createInput"
          v-model="newName"
          size="small"
          placeholder="分类名称，回车保存"
          :maxlength="NAME_MAX"
          :disabled="saving"
          @keyup.enter="submitCreate"
          @keyup.esc="creating = false"
          @blur="submitCreate"
        />
      </div>

      <div
        v-if="!loading && categories.length === 0 && !creating"
        class="px-3 py-4 text-center text-xs text-[var(--el-text-color-secondary)]"
      >
        暂无分类
        <template v-if="canEdit">
          ，
          <el-button link type="primary" size="small" @click="startCreate">
            新建一个
          </el-button>
        </template>
      </div>
    </div>
  </el-card>
</template>

<style scoped lang="scss">
.category-list {
  min-height: 120px;
}

.category-item {
  display: flex;
  gap: 8px;
  align-items: center;
  height: 36px;
  padding: 0 10px;
  font-size: 14px;
  color: var(--el-text-color-regular);
  cursor: pointer;
  border-radius: 6px;
  transition:
    background-color 0.15s,
    color 0.15s;

  &:hover {
    background-color: var(--el-fill-color-light);
  }

  &.is-active {
    font-weight: 500;
    color: var(--el-color-primary);
    background-color: var(--el-color-primary-light-9);
  }
}

.category-item__icon {
  flex-shrink: 0;
  width: 16px;
  height: 16px;
}

.category-item__name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.category-item__count {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.category-item__actions {
  align-items: center;

  :deep(.el-button + .el-button) {
    margin-left: 4px;
  }
}
</style>
