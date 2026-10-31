<script setup lang="ts">
import { DEPT_TREE_PROPS } from "@/utils/tree-options";
import { computed, reactive, ref, watch } from "vue";
import type { FormInstance, FormRules } from "element-plus";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";
import { useFieldErrors } from "@/composables/useFieldErrors";
import {
  STATUS_ENABLED,
  STATUS_OPTIONS,
  type CodeOption,
  type Status
} from "@/api/types";
import type { Dept } from "@/api/iam/dept";
import { getRoleOptions } from "@/api/iam/role";
import { getPositionOptions } from "@/api/iam/position";
import { createUser, updateUser, type User } from "@/api/iam/user";
import { PASSWORD_MAX, PASSWORD_MIN } from "@/views/login/utils/rule";

defineOptions({ name: "UserFormDialog" });

const props = defineProps<{ user: User | null; deptTree: Dept[] }>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{
  (e: "saved"): void;
  (e: "reload"): void;
}>();

const USERNAME_PATTERN = /^[a-zA-Z0-9_.-]{3,32}$/;

const formRef = ref<FormInstance>();
const saving = ref(false);
const roleOptions = ref<CodeOption[]>([]);
const positionOptions = ref<CodeOption[]>([]);
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();

const isEdit = computed(() => props.user !== null);

type UserForm = {
  username: string;
  password: string;
  nickname: string;
  email: string;
  phone: string;
  deptId: string | null;
  status: Status;
  roleIds: string[];
  positionIds: string[];
};

const emptyForm = (): UserForm => ({
  username: "",
  password: "",
  nickname: "",
  email: "",
  phone: "",
  deptId: null,
  status: STATUS_ENABLED,
  roleIds: [],
  positionIds: []
});

const form = reactive<UserForm>(emptyForm());

const rules = computed<FormRules>(() => ({
  username: isEdit.value
    ? []
    : [
        { required: true, message: "请输入用户名", trigger: "blur" },
        {
          pattern: USERNAME_PATTERN,
          message: "3–32 位字母、数字或 _ . -",
          trigger: "blur"
        }
      ],
  password: isEdit.value
    ? []
    : [
        { required: true, message: "请输入初始密码", trigger: "blur" },
        {
          min: PASSWORD_MIN,
          max: PASSWORD_MAX,
          message: `密码长度为 ${PASSWORD_MIN}–${PASSWORD_MAX} 位`,
          trigger: "blur"
        }
      ],
  nickname: [{ required: true, message: "请输入昵称", trigger: "blur" }],
  email: [{ type: "email", message: "邮箱格式不正确", trigger: "blur" }]
}));

async function loadOptions(): Promise<void> {
  const [roles, positions] = await Promise.allSettled([
    getRoleOptions(),
    getPositionOptions()
  ]);
  roleOptions.value = roles.status === "fulfilled" ? roles.value : [];
  positionOptions.value =
    positions.status === "fulfilled" ? positions.value : [];
}

watch(visible, open => {
  if (!open) return;
  clearFieldErrors();
  const u = props.user;
  Object.assign(
    form,
    u
      ? {
          ...emptyForm(),
          username: u.username,
          nickname: u.nickname,
          email: u.email,
          phone: u.phone,
          deptId: u.deptId,
          status: u.status,
          roleIds: [...u.roleIds],
          positionIds: [...u.positionIds]
        }
      : emptyForm()
  );
  formRef.value?.clearValidate();
  loadOptions();
});

async function submit(): Promise<void> {
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;
  clearFieldErrors();
  saving.value = true;
  try {
    if (props.user) {
      await updateUser(props.user.id, {
        nickname: form.nickname,
        email: form.email,
        phone: form.phone,
        deptId: form.deptId || null,
        roleIds: form.roleIds,
        positionIds: form.positionIds,
        version: props.user.version
      });
    } else {
      await createUser({ ...form, deptId: form.deptId || null });
    }
    message(isEdit.value ? "用户已更新" : "用户已创建", { type: "success" });
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
    :title="isEdit ? '编辑用户' : '新增用户'"
    width="640px"
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
      <el-row :gutter="16">
        <el-col :span="12">
          <el-form-item
            label="用户名"
            prop="username"
            :error="fieldErrors.username"
          >
            <el-input
              v-model="form.username"
              :disabled="isEdit"
              maxlength="32"
            />
          </el-form-item>
        </el-col>
        <el-col v-if="!isEdit" :span="12">
          <el-form-item
            label="初始密码"
            prop="password"
            :error="fieldErrors.password"
          >
            <el-input
              v-model="form.password"
              type="password"
              show-password
              autocomplete="new-password"
            />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item
            label="昵称"
            prop="nickname"
            :error="fieldErrors.nickname"
          >
            <el-input v-model="form.nickname" maxlength="64" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="部门" prop="deptId" :error="fieldErrors.deptId">
            <el-tree-select
              v-model="form.deptId"
              :data="deptTree"
              :props="DEPT_TREE_PROPS"
              node-key="id"
              check-strictly
              clearable
              filterable
              default-expand-all
              placeholder="选择部门"
              class="w-full"
            />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="邮箱" prop="email" :error="fieldErrors.email">
            <el-input v-model="form.email" maxlength="128" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="手机号" prop="phone" :error="fieldErrors.phone">
            <el-input v-model="form.phone" maxlength="32" />
          </el-form-item>
        </el-col>
        <el-col :span="24">
          <el-form-item
            label="角色"
            prop="roleIds"
            :error="fieldErrors.roleIds"
          >
            <el-select
              v-model="form.roleIds"
              multiple
              filterable
              placeholder="选择角色"
              class="w-full"
            >
              <el-option
                v-for="r in roleOptions"
                :key="r.id"
                :label="r.name"
                :value="r.id"
              />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :span="24">
          <el-form-item
            label="岗位"
            prop="positionIds"
            :error="fieldErrors.positionIds"
          >
            <el-select
              v-model="form.positionIds"
              multiple
              filterable
              placeholder="选择岗位"
              class="w-full"
            >
              <el-option
                v-for="p in positionOptions"
                :key="p.id"
                :label="p.name"
                :value="p.id"
              />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col v-if="!isEdit" :span="24">
          <el-form-item label="状态" prop="status">
            <el-radio-group v-model="form.status">
              <el-radio
                v-for="o in STATUS_OPTIONS"
                :key="o.value"
                :value="o.value"
              >
                {{ o.label }}
              </el-radio>
            </el-radio-group>
          </el-form-item>
        </el-col>
      </el-row>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="submit">
        保存
      </el-button>
    </template>
  </el-dialog>
</template>
