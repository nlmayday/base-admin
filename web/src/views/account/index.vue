<script setup lang="ts">
import { reactive, ref, watch } from "vue";
import type { FormInstance, FormRules } from "element-plus";
import { message } from "@/utils/message";
import { updateMyProfile } from "@/api/auth";
import { needsReload } from "@/utils/http/errors";
import { useUserStoreHook } from "@/store/modules/user";
import { useFieldErrors } from "@/composables/useFieldErrors";
import PasswordForm from "./components/PasswordForm.vue";
import ReImageUpload from "@/components/ReUpload/ImageUpload.vue";

defineOptions({ name: "AccountCenter" });

const userStore = useUserStoreHook();
const activeTab = ref<"profile" | "password">("profile");
const formRef = ref<FormInstance>();
const saving = ref(false);
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();

const form = reactive({
  nickname: "",
  email: "",
  phone: "",
  avatarUrl: "",
  version: 0
});

function fillFromStore(): void {
  const user = userStore.me?.user;
  if (!user) return;
  Object.assign(form, {
    nickname: user.nickname,
    email: user.email,
    phone: user.phone,
    avatarUrl: user.avatarUrl,
    version: user.version
  });
}

watch(() => userStore.me, fillFromStore, { immediate: true });

const rules: FormRules = {
  nickname: [{ required: true, message: "请输入昵称", trigger: "blur" }],
  email: [{ type: "email", message: "邮箱格式不正确", trigger: "blur" }]
};

async function saveProfile(): Promise<void> {
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;
  clearFieldErrors();
  saving.value = true;
  try {
    const me = await updateMyProfile({ ...form });
    userStore.setMe(me);
    message("个人信息已保存", { type: "success" });
  } catch (err) {
    applyFieldErrors(err);
    if (needsReload(err)) {
      await userStore.fetchMe().catch(() => undefined);
    }
  } finally {
    saving.value = false;
  }
}

function onPasswordChanged(): void {
  message("密码已修改，其他设备已下线", { type: "success" });
}
</script>

<template>
  <div class="main">
    <el-card shadow="never">
      <el-tabs v-model="activeTab">
        <el-tab-pane label="基本信息" name="profile">
          <el-descriptions :column="2" border class="mb-6 max-w-[720px]">
            <el-descriptions-item label="用户名">
              {{ userStore.me?.user.username }}
            </el-descriptions-item>
            <el-descriptions-item label="部门">
              {{ userStore.me?.user.deptName || "-" }}
            </el-descriptions-item>
            <el-descriptions-item label="角色" :span="2">
              <el-tag
                v-for="role in userStore.me?.roles ?? []"
                :key="role.id"
                class="mr-1"
              >
                {{ role.name }}
              </el-tag>
            </el-descriptions-item>
          </el-descriptions>
          <el-form
            ref="formRef"
            :model="form"
            :rules="rules"
            label-width="100px"
            class="max-w-[480px]"
          >
            <el-form-item
              label="昵称"
              prop="nickname"
              :error="fieldErrors.nickname"
            >
              <el-input v-model="form.nickname" maxlength="64" />
            </el-form-item>
            <el-form-item label="邮箱" prop="email" :error="fieldErrors.email">
              <el-input v-model="form.email" maxlength="128" />
            </el-form-item>
            <el-form-item
              label="手机号"
              prop="phone"
              :error="fieldErrors.phone"
            >
              <el-input v-model="form.phone" maxlength="32" />
            </el-form-item>
            <el-form-item
              label="头像"
              prop="avatarUrl"
              :error="fieldErrors.avatarUrl"
            >
              <ReImageUpload
                v-model="form.avatarUrl"
                :size="72"
                tip="支持常见图片格式，保存后生效"
              />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="saving" @click="saveProfile">
                保存
              </el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>
        <el-tab-pane label="修改密码" name="password">
          <PasswordForm @success="onPasswordChanged" />
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>
