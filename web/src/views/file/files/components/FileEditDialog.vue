<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import type { FormInstance, FormRules } from "element-plus";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";
import { useFieldErrors } from "@/composables/useFieldErrors";
import { updateFile, type FileCategory, type FileItem } from "@/api/file";
import { splitName } from "../utils";

defineOptions({ name: "FileEditDialog" });

export type EditMode = "rename" | "move";

const props = defineProps<{
  file: FileItem | null;
  mode: EditMode;
  categories: FileCategory[];
}>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{
  (e: "saved", file: FileItem): void;
  (e: "reload"): void;
}>();

const NAME_MAX = 255;

const formRef = ref<FormInstance>();
const saving = ref(false);
const form = reactive({ base: "", categoryId: null as string | null });
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();

const ext = computed(() => props.file?.ext ?? "");
const title = computed(() =>
  props.mode === "rename" ? "重命名" : "移动到分类"
);

const rules = computed<FormRules>(() =>
  props.mode === "rename"
    ? {
        base: [
          { required: true, message: "请输入文件名", trigger: "blur" },
          {
            validator: (_r, value: string, cb) =>
              /[\\/]/.test(value)
                ? cb(new Error("文件名不能包含 / 或 \\"))
                : cb(),
            trigger: "blur"
          }
        ]
      }
    : {}
);

watch(visible, open => {
  if (!open || !props.file) return;
  clearFieldErrors();
  const { base, ext: current } = splitName(props.file.originalName);
  // 扩展名由服务端保留，这里只编辑主文件名
  form.base =
    current.toLowerCase() === ext.value ? base : props.file.originalName;
  form.categoryId = props.file.categoryId;
  formRef.value?.clearValidate();
});

async function submit(): Promise<void> {
  const file = props.file;
  if (!file) return;
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;
  clearFieldErrors();
  saving.value = true;
  try {
    const name = form.base.trim();
    const saved = await updateFile(file.id, {
      originalName:
        props.mode === "rename" ? `${name}.${ext.value}` : file.originalName,
      categoryId: props.mode === "move" ? form.categoryId : file.categoryId,
      version: file.version
    });
    message(props.mode === "rename" ? "已重命名" : "已移动", {
      type: "success"
    });
    visible.value = false;
    emit("saved", saved);
  } catch (err) {
    if (applyFieldErrors(err)) {
      fieldErrors.value.base = fieldErrors.value.originalName;
    }
    if (needsReload(err)) {
      visible.value = false;
      emit("reload");
    }
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="title"
    width="460px"
    destroy-on-close
    :close-on-click-modal="false"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      :validate-on-rule-change="false"
      label-width="80px"
      @submit.prevent="submit"
    >
      <el-form-item
        v-if="mode === 'rename'"
        label="文件名"
        prop="base"
        :error="fieldErrors.base"
      >
        <el-input
          v-model="form.base"
          :maxlength="NAME_MAX - ext.length - 1"
          show-word-limit
          @keyup.enter="submit"
        >
          <template v-if="ext" #append>.{{ ext }}</template>
        </el-input>
      </el-form-item>
      <el-form-item
        v-else
        label="目标分类"
        prop="categoryId"
        :error="fieldErrors.categoryId"
      >
        <el-select
          v-model="form.categoryId"
          placeholder="未分类"
          clearable
          filterable
          class="w-full"
        >
          <el-option
            v-for="c in categories"
            :key="c.id"
            :label="c.name"
            :value="c.id"
          />
        </el-select>
        <div class="mt-1 text-xs text-[var(--el-text-color-secondary)]">
          清空选择即移出分类
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="submit">
        保存
      </el-button>
    </template>
  </el-dialog>
</template>
