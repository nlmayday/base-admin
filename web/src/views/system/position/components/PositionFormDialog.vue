<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import type { FormInstance, FormRules } from "element-plus";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";
import { useFieldErrors } from "@/composables/useFieldErrors";
import { STATUS_ENABLED, STATUS_OPTIONS, type Status } from "@/api/types";
import {
  createPosition,
  updatePosition,
  type Position
} from "@/api/iam/position";

defineOptions({ name: "PositionFormDialog" });

const props = defineProps<{ position: Position | null }>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{ (e: "saved"): void; (e: "reload"): void }>();

type PositionForm = {
  code: string;
  name: string;
  sort: number;
  status: Status;
  remark: string;
};

const emptyForm = (): PositionForm => ({
  code: "",
  name: "",
  sort: 0,
  status: STATUS_ENABLED,
  remark: ""
});

const formRef = ref<FormInstance>();
const saving = ref(false);
const form = reactive<PositionForm>(emptyForm());
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();
const isEdit = computed(() => props.position !== null);

const rules = computed<FormRules>(() => ({
  code: isEdit.value
    ? []
    : [{ required: true, message: "请输入岗位编码", trigger: "blur" }],
  name: [{ required: true, message: "请输入岗位名称", trigger: "blur" }]
}));

watch(visible, open => {
  if (!open) return;
  clearFieldErrors();
  const p = props.position;
  Object.assign(
    form,
    p
      ? {
          code: p.code,
          name: p.name,
          sort: p.sort,
          status: p.status,
          remark: p.remark
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
    if (props.position) {
      await updatePosition(props.position.id, {
        name: form.name,
        sort: form.sort,
        status: form.status,
        remark: form.remark,
        version: props.position.version
      });
    } else {
      await createPosition({ ...form });
    }
    message(isEdit.value ? "岗位已更新" : "岗位已创建", { type: "success" });
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
    :title="isEdit ? '编辑岗位' : '新增岗位'"
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
      <el-form-item label="岗位编码" prop="code" :error="fieldErrors.code">
        <el-input v-model="form.code" :disabled="isEdit" maxlength="32" />
      </el-form-item>
      <el-form-item label="岗位名称" prop="name" :error="fieldErrors.name">
        <el-input v-model="form.name" maxlength="64" />
      </el-form-item>
      <el-form-item label="排序" prop="sort" :error="fieldErrors.sort">
        <el-input-number v-model="form.sort" :min="0" :max="9999" />
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
