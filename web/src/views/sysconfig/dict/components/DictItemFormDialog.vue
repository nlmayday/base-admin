<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import type { FormInstance, FormRules } from "element-plus";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";
import { useFieldErrors } from "@/composables/useFieldErrors";
import { STATUS_ENABLED, STATUS_OPTIONS, type Status } from "@/api/types";
import {
  createDictItem,
  DICT_TAG_TYPES,
  updateDictItem,
  type DictItem,
  type DictTagType
} from "@/api/system/dict";

defineOptions({ name: "DictItemFormDialog" });

const props = defineProps<{ typeId: string; item: DictItem | null }>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{ (e: "saved"): void; (e: "reload"): void }>();

type DictItemForm = {
  label: string;
  value: string;
  sort: number;
  status: Status;
  tagType: DictTagType;
  remark: string;
};

const emptyForm = (): DictItemForm => ({
  label: "",
  value: "",
  sort: 0,
  status: STATUS_ENABLED,
  tagType: "",
  remark: ""
});

const formRef = ref<FormInstance>();
const saving = ref(false);
const form = reactive<DictItemForm>(emptyForm());
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();
const isEdit = computed(() => props.item !== null);

const rules: FormRules = {
  label: [{ required: true, message: "请输入字典标签", trigger: "blur" }],
  value: [{ required: true, message: "请输入字典值", trigger: "blur" }]
};

watch(visible, open => {
  if (!open) return;
  clearFieldErrors();
  const i = props.item;
  Object.assign(
    form,
    i
      ? {
          label: i.label,
          value: i.value,
          sort: i.sort,
          status: i.status,
          tagType: i.tagType,
          remark: i.remark
        }
      : emptyForm()
  );
  formRef.value?.clearValidate();
});

async function submit(): Promise<void> {
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;
  clearFieldErrors();
  saving.value = true;
  try {
    if (props.item) {
      await updateDictItem(props.item.id, {
        ...form,
        version: props.item.version
      });
    } else {
      await createDictItem(props.typeId, { ...form });
    }
    message(isEdit.value ? "字典项已更新" : "字典项已创建", {
      type: "success"
    });
    visible.value = false;
    emit("saved");
  } catch (err) {
    applyFieldErrors(err);
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
    :title="isEdit ? '编辑字典项' : '新增字典项'"
    width="520px"
    destroy-on-close
    :close-on-click-modal="false"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      :validate-on-rule-change="false"
      label-width="90px"
    >
      <el-form-item label="字典标签" prop="label" :error="fieldErrors.label">
        <el-input v-model="form.label" maxlength="64" placeholder="显示文字" />
      </el-form-item>
      <el-form-item label="字典值" prop="value" :error="fieldErrors.value">
        <el-input v-model="form.value" maxlength="64" placeholder="存储的值" />
      </el-form-item>
      <el-form-item
        label="标签样式"
        prop="tagType"
        :error="fieldErrors.tagType"
      >
        <div class="flex flex-wrap items-center gap-2">
          <el-radio-group v-model="form.tagType" size="small">
            <el-radio-button
              v-for="o in DICT_TAG_TYPES"
              :key="o.value"
              :value="o.value"
            >
              {{ o.label }}
            </el-radio-button>
          </el-radio-group>
          <el-tag
            :type="form.tagType || undefined"
            disable-transitions
            class="ml-1"
          >
            {{ form.label || "预览" }}
          </el-tag>
        </div>
      </el-form-item>
      <el-form-item label="排序" prop="sort" :error="fieldErrors.sort">
        <el-input-number v-model="form.sort" :min="0" :max="99999" />
      </el-form-item>
      <el-form-item label="状态" prop="status">
        <el-radio-group v-model="form.status">
          <el-radio v-for="o in STATUS_OPTIONS" :key="o.value" :value="o.value">
            {{ o.label }}
          </el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="备注" prop="remark" :error="fieldErrors.remark">
        <el-input v-model="form.remark" type="textarea" maxlength="255" />
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
