<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import type { FormInstance, FormRules } from "element-plus";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";
import { useFieldErrors } from "@/composables/useFieldErrors";
import { STATUS_ENABLED, STATUS_OPTIONS, type Status } from "@/api/types";
import {
  createDictType,
  updateDictType,
  type DictType
} from "@/api/system/dict";

defineOptions({ name: "DictTypeFormDialog" });

const props = defineProps<{ dictType: DictType | null }>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{
  (e: "saved", value: DictType): void;
  (e: "reload"): void;
}>();

type DictTypeForm = {
  code: string;
  name: string;
  status: Status;
  remark: string;
};

const CODE_PATTERN = /^[a-z][a-z0-9_]{1,63}$/;

const emptyForm = (): DictTypeForm => ({
  code: "",
  name: "",
  status: STATUS_ENABLED,
  remark: ""
});

const formRef = ref<FormInstance>();
const saving = ref(false);
const form = reactive<DictTypeForm>(emptyForm());
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();
const isEdit = computed(() => props.dictType !== null);

const rules = computed<FormRules>(() => ({
  code: isEdit.value
    ? []
    : [
        { required: true, message: "请输入字典编码", trigger: "blur" },
        {
          pattern: CODE_PATTERN,
          message: "小写字母开头，2–64 位小写字母、数字或下划线",
          trigger: "blur"
        }
      ],
  name: [{ required: true, message: "请输入字典名称", trigger: "blur" }]
}));

watch(visible, open => {
  if (!open) return;
  clearFieldErrors();
  const t = props.dictType;
  Object.assign(
    form,
    t
      ? { code: t.code, name: t.name, status: t.status, remark: t.remark }
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
    const saved = props.dictType
      ? await updateDictType(props.dictType.id, {
          name: form.name,
          status: form.status,
          remark: form.remark,
          version: props.dictType.version
        })
      : await createDictType({ ...form });
    message(isEdit.value ? "字典类型已更新" : "字典类型已创建", {
      type: "success"
    });
    visible.value = false;
    emit("saved", saved);
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
    :title="isEdit ? '编辑字典类型' : '新增字典类型'"
    width="480px"
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
      <el-form-item label="字典编码" prop="code" :error="fieldErrors.code">
        <el-input
          v-model="form.code"
          :disabled="isEdit"
          maxlength="64"
          placeholder="如 order_status"
        />
      </el-form-item>
      <el-form-item label="字典名称" prop="name" :error="fieldErrors.name">
        <el-input
          v-model="form.name"
          maxlength="64"
          placeholder="如 订单状态"
        />
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
