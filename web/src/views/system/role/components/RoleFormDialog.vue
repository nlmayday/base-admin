<script setup lang="ts">
import { DEPT_TREE_PROPS } from "@/utils/tree-options";
import { computed, reactive, ref, watch } from "vue";
import type { FormInstance, FormRules } from "element-plus";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";
import { useFieldErrors } from "@/composables/useFieldErrors";
import { STATUS_ENABLED, STATUS_OPTIONS, type Status } from "@/api/types";
import type { Dept } from "@/api/iam/dept";
import {
  DATA_SCOPE_OPTIONS,
  createRole,
  updateRole,
  type DataScope,
  type Role
} from "@/api/iam/role";

defineOptions({ name: "RoleFormDialog" });

const props = defineProps<{ role: Role | null; deptTree: Dept[] }>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{ (e: "saved"): void; (e: "reload"): void }>();

const ROLE_CODE_PATTERN = /^[a-z][a-z0-9_]{1,31}$/;

type RoleForm = {
  code: string;
  name: string;
  dataScope: DataScope;
  deptIds: string[];
  status: Status;
  sort: number;
  remark: string;
};

const emptyForm = (): RoleForm => ({
  code: "",
  name: "",
  dataScope: "self",
  deptIds: [],
  status: STATUS_ENABLED,
  sort: 0,
  remark: ""
});

const formRef = ref<FormInstance>();
const saving = ref(false);
const form = reactive<RoleForm>(emptyForm());
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();
const isEdit = computed(() => props.role !== null);

const rules = computed<FormRules>(() => ({
  code: isEdit.value
    ? []
    : [
        { required: true, message: "请输入角色编码", trigger: "blur" },
        {
          pattern: ROLE_CODE_PATTERN,
          message: "小写字母开头，2–32 位小写字母、数字或下划线",
          trigger: "blur"
        }
      ],
  name: [{ required: true, message: "请输入角色名称", trigger: "blur" }],
  deptIds:
    form.dataScope === "custom"
      ? [
          {
            type: "array",
            required: true,
            message: "请选择部门",
            trigger: "change"
          }
        ]
      : []
}));

watch(visible, open => {
  if (!open) return;
  clearFieldErrors();
  const r = props.role;
  Object.assign(
    form,
    r
      ? {
          code: r.code,
          name: r.name,
          dataScope: r.dataScope,
          deptIds: [...(r.deptIds ?? [])],
          status: r.status,
          sort: r.sort,
          remark: r.remark
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
  const deptIds = form.dataScope === "custom" ? form.deptIds : [];
  try {
    if (props.role) {
      await updateRole(props.role.id, {
        name: form.name,
        dataScope: form.dataScope,
        deptIds,
        status: form.status,
        sort: form.sort,
        remark: form.remark,
        version: props.role.version
      });
    } else {
      await createRole({ ...form, deptIds });
    }
    message(isEdit.value ? "角色已更新" : "角色已创建", { type: "success" });
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
    :title="isEdit ? '编辑角色' : '新增角色'"
    width="560px"
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
      <el-form-item label="角色编码" prop="code" :error="fieldErrors.code">
        <el-input v-model="form.code" :disabled="isEdit" maxlength="32" />
      </el-form-item>
      <el-form-item label="角色名称" prop="name" :error="fieldErrors.name">
        <el-input v-model="form.name" maxlength="64" />
      </el-form-item>
      <el-form-item
        label="数据范围"
        prop="dataScope"
        :error="fieldErrors.dataScope"
      >
        <el-select v-model="form.dataScope" class="w-full">
          <el-option
            v-for="o in DATA_SCOPE_OPTIONS"
            :key="o.value"
            :label="o.label"
            :value="o.value"
          />
        </el-select>
      </el-form-item>
      <el-form-item
        v-if="form.dataScope === 'custom'"
        label="部门"
        prop="deptIds"
        :error="fieldErrors.deptIds"
      >
        <el-tree-select
          v-model="form.deptIds"
          :data="deptTree"
          :props="DEPT_TREE_PROPS"
          node-key="id"
          multiple
          show-checkbox
          check-strictly
          filterable
          default-expand-all
          placeholder="选择部门"
          class="w-full"
        />
      </el-form-item>
      <el-form-item label="状态" prop="status">
        <el-radio-group v-model="form.status">
          <el-radio v-for="o in STATUS_OPTIONS" :key="o.value" :value="o.value">
            {{ o.label }}
          </el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="排序" prop="sort" :error="fieldErrors.sort">
        <el-input-number v-model="form.sort" :min="0" :max="9999" />
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
