<script setup lang="ts">
import { reactive, ref } from "vue";
import type { FormInstance, FormRules } from "element-plus";
import { changeMyPassword } from "@/api/auth";
import { useFieldErrors } from "@/composables/useFieldErrors";
import { PASSWORD_MAX, PASSWORD_MIN } from "@/views/login/utils/rule";

defineOptions({ name: "PasswordForm" });

const emit = defineEmits<{ (e: "success"): void }>();

const formRef = ref<FormInstance>();
const submitting = ref(false);
const form = reactive({ oldPassword: "", newPassword: "", confirm: "" });
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();

const rules: FormRules = {
  oldPassword: [{ required: true, message: "请输入当前密码", trigger: "blur" }],
  newPassword: [
    { required: true, message: "请输入新密码", trigger: "blur" },
    {
      min: PASSWORD_MIN,
      max: PASSWORD_MAX,
      message: `密码长度为 ${PASSWORD_MIN}–${PASSWORD_MAX} 位`,
      trigger: "blur"
    }
  ],
  confirm: [
    { required: true, message: "请再次输入新密码", trigger: "blur" },
    {
      validator: (_rule, value, callback) =>
        value === form.newPassword
          ? callback()
          : callback(new Error("两次输入的密码不一致")),
      trigger: "blur"
    }
  ]
};

async function submit(): Promise<void> {
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;
  clearFieldErrors();
  submitting.value = true;
  try {
    await changeMyPassword({
      oldPassword: form.oldPassword,
      newPassword: form.newPassword
    });
    formRef.value?.resetFields();
    emit("success");
  } catch (err) {
    applyFieldErrors(err);
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <el-form
    ref="formRef"
    :model="form"
    :rules="rules"
    label-width="100px"
    class="max-w-[480px]"
  >
    <el-form-item
      label="当前密码"
      prop="oldPassword"
      :error="fieldErrors.oldPassword"
    >
      <el-input
        v-model="form.oldPassword"
        data-testid="pwd-old"
        type="password"
        show-password
        autocomplete="current-password"
      />
    </el-form-item>
    <el-form-item
      label="新密码"
      prop="newPassword"
      :error="fieldErrors.newPassword"
    >
      <el-input
        v-model="form.newPassword"
        data-testid="pwd-new"
        type="password"
        show-password
        autocomplete="new-password"
      />
    </el-form-item>
    <el-form-item label="确认新密码" prop="confirm">
      <el-input
        v-model="form.confirm"
        data-testid="pwd-confirm"
        type="password"
        show-password
        autocomplete="new-password"
      />
    </el-form-item>
    <el-form-item>
      <el-button
        type="primary"
        data-testid="pwd-submit"
        :loading="submitting"
        @click="submit"
      >
        修改密码
      </el-button>
    </el-form-item>
  </el-form>
</template>
