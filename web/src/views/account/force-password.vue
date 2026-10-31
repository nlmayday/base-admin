<script setup lang="ts">
import { useRoute, useRouter } from "vue-router";
import { message } from "@/utils/message";
import { useUserStoreHook } from "@/store/modules/user";
import { safeRedirect } from "@/router/paths";
import PasswordForm from "./components/PasswordForm.vue";

defineOptions({ name: "ForceChangePassword" });

const route = useRoute();
const router = useRouter();
const userStore = useUserStoreHook();

/** 改密成功后后端签发新会话；重新拉取 /me 后进入系统 */
async function onSuccess(): Promise<void> {
  message("密码已修改", { type: "success" });
  try {
    await userStore.fetchMe();
  } catch (err) {
    console.error("[change-password] reload /me failed", err);
    userStore.clearSession();
  }
  await router.replace(safeRedirect(route.query.redirect));
}
</script>

<template>
  <div
    class="flex min-h-screen items-center justify-center bg-[var(--el-bg-color-page)]"
  >
    <el-card class="w-[560px]" shadow="never">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-medium">首次登录或密码已重置，请先修改密码</span>
          <el-button link type="primary" @click="userStore.logout()">
            退出登录
          </el-button>
        </div>
      </template>
      <PasswordForm @success="onSuccess" />
    </el-card>
  </div>
</template>
