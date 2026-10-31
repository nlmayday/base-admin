<script setup lang="ts">
import Motion from "./utils/motion";
import { useRoute, useRouter } from "vue-router";
import { message } from "@/utils/message";
import { loginRules } from "./utils/rule";
import { onMounted, ref, reactive, toRaw } from "vue";
import { debounce } from "@pureadmin/utils";
import { useNav } from "@/layout/hooks/useNav";
import { useEventListener } from "@vueuse/core";
import type { FormInstance } from "element-plus";
import { useLayout } from "@/layout/hooks/useLayout";
import { useUserStoreHook } from "@/store/modules/user";
import { bg, avatar, illustration } from "./utils/static";
import { useRenderIcon } from "@/components/ReIcon/src/hooks";
import { useDataThemeChange } from "@/layout/hooks/useDataThemeChange";
import { ErrorCode, hasErrorCode, isApiError } from "@/utils/http/errors";
import { FORCE_PASSWORD_PATH, safeRedirect } from "@/router/paths";
import { useSettingStoreHook } from "@/store/modules/settings";
import { getCaptcha, getLoginOptions, type LoginFailureData } from "@/api/auth";

import dayIcon from "@/assets/svg/day.svg?component";
import darkIcon from "@/assets/svg/dark.svg?component";
import Lock from "~icons/ri/lock-fill";
import User from "~icons/ri/user-3-fill";
import ShieldKey from "~icons/ri/shield-keyhole-fill";

defineOptions({
  name: "Login"
});

const router = useRouter();
const route = useRoute();
const loading = ref(false);
const disabled = ref(false);
const ruleFormRef = ref<FormInstance>();
/** 账号或密码错误（20004）等需要展示在表单上的错误 */
const loginError = ref("");

const { initStorage } = useLayout();
initStorage();

const { dataTheme, overallStyle, dataThemeChange } = useDataThemeChange();
dataThemeChange(overallStyle.value);
const { title } = useNav();
const settings = useSettingStoreHook();

const ruleForm = reactive({
  username: "",
  password: "",
  captchaCode: ""
});

/** 验证码：安全策略要求时显示（始终需要，或失败次数达到阈值后由后端告知） */
const captcha = reactive({ visible: false, id: "", image: "", error: "" });

async function refreshCaptcha(): Promise<void> {
  try {
    const c = await getCaptcha();
    captcha.id = c.id;
    captcha.image = c.image;
    ruleForm.captchaCode = "";
  } catch {
    captcha.error = "验证码加载失败，点击图片重试";
  }
}

function requireCaptcha(error = ""): void {
  captcha.visible = true;
  captcha.error = error;
  refreshCaptcha();
}

onMounted(async () => {
  try {
    if ((await getLoginOptions()).captchaAlways) requireCaptcha();
  } catch {
    // 登录选项不可用时按无需验证码处理，后端仍会校验
  }
});

/** 显示在登录表单上的拒绝：账号锁定、IP 或时段限制、业务规则 */
const REFUSAL_CODES = [
  ErrorCode.ACCOUNT_LOCKED,
  ErrorCode.LOGIN_IP_DENIED,
  ErrorCode.LOGIN_TIME_DENIED,
  ErrorCode.LOGIN_DENIED
];

/** 登录失败后的界面反馈：拒绝登录、验证码、账号或密码错误 */
function showLoginFailure(err: unknown): void {
  if (isApiError(err) && REFUSAL_CODES.some(code => code === err.code)) {
    loginError.value = err.message;
  } else if (hasErrorCode(err, ErrorCode.CAPTCHA_REQUIRED)) {
    requireCaptcha(captcha.visible ? err.message : "请输入验证码");
  } else if (hasErrorCode(err, ErrorCode.BAD_CREDENTIALS)) {
    loginError.value = err.message || "账号或密码错误";
    const data = err.data as LoginFailureData | null;
    if (data?.captchaRequired || captcha.visible) requireCaptcha();
  } else if (!isApiError(err)) {
    console.error("[login] unexpected error", err);
  } else if (captcha.visible) {
    refreshCaptcha();
  }
}

async function submit(): Promise<void> {
  loading.value = true;
  loginError.value = "";
  captcha.error = "";
  try {
    const me = await useUserStoreHook().login({
      username: ruleForm.username.trim(),
      password: ruleForm.password,
      ...(captcha.visible
        ? { captchaId: captcha.id, captchaCode: ruleForm.captchaCode.trim() }
        : {})
    });
    const redirect = safeRedirect(route.query.redirect);
    disabled.value = true;
    if (me.mustChangePassword) {
      await router.replace({ path: FORCE_PASSWORD_PATH, query: { redirect } });
      return;
    }
    // 动态路由由路由守卫在首次进入时从 /me/menus 加载
    await router.replace(redirect);
    message("登录成功", { type: "success" });
  } catch (err) {
    showLoginFailure(err);
  } finally {
    loading.value = false;
    disabled.value = false;
  }
}

const onLogin = async (formEl: FormInstance | undefined) => {
  if (!formEl) return;
  const valid = await formEl.validate().catch(() => false);
  if (valid) await submit();
};

const immediateDebounce: any = debounce(
  formRef => onLogin(formRef),
  1000,
  true
);

useEventListener(document, "keydown", ({ code }) => {
  if (
    ["Enter", "NumpadEnter"].includes(code) &&
    !disabled.value &&
    !loading.value
  )
    immediateDebounce(ruleFormRef.value);
});
</script>

<template>
  <div class="select-none">
    <img :src="bg" class="wave" />
    <div class="flex-c absolute right-5 top-3">
      <!-- 主题 -->
      <el-switch
        v-model="dataTheme"
        inline-prompt
        :active-icon="dayIcon"
        :inactive-icon="darkIcon"
        @change="dataThemeChange"
      />
    </div>
    <div class="login-container">
      <div class="img">
        <component :is="toRaw(illustration)" />
      </div>
      <div class="login-box">
        <div class="login-form">
          <img
            v-if="settings.logo"
            :src="settings.logo"
            alt="logo"
            class="avatar site-logo"
          />
          <avatar v-else class="avatar" />
          <Motion>
            <h2 class="outline-hidden">{{ title }}</h2>
          </Motion>

          <el-form
            ref="ruleFormRef"
            :model="ruleForm"
            :rules="loginRules"
            size="large"
          >
            <Motion :delay="100">
              <el-form-item
                :rules="[
                  {
                    required: true,
                    message: '请输入账号',
                    trigger: 'blur'
                  }
                ]"
                prop="username"
              >
                <el-input
                  v-model="ruleForm.username"
                  data-testid="login-username"
                  clearable
                  autocomplete="username"
                  placeholder="账号"
                  :prefix-icon="useRenderIcon(User)"
                />
              </el-form-item>
            </Motion>

            <Motion :delay="150">
              <el-form-item prop="password" :error="loginError">
                <el-input
                  v-model="ruleForm.password"
                  data-testid="login-password"
                  clearable
                  show-password
                  autocomplete="current-password"
                  placeholder="密码"
                  :prefix-icon="useRenderIcon(Lock)"
                />
              </el-form-item>
            </Motion>

            <Motion v-if="captcha.visible" :delay="50">
              <el-form-item
                prop="captchaCode"
                :error="captcha.error"
                :rules="[
                  { required: true, message: '请输入验证码', trigger: 'blur' }
                ]"
              >
                <div class="captcha-row">
                  <el-input
                    v-model="ruleForm.captchaCode"
                    data-testid="login-captcha"
                    clearable
                    maxlength="8"
                    autocomplete="off"
                    placeholder="验证码"
                    :prefix-icon="useRenderIcon(ShieldKey)"
                  />
                  <img
                    v-if="captcha.image"
                    :src="captcha.image"
                    class="captcha-img"
                    alt="验证码"
                    title="看不清？点击换一张"
                    @click="refreshCaptcha"
                  />
                </div>
              </el-form-item>
            </Motion>

            <Motion :delay="250">
              <el-button
                data-testid="login-submit"
                class="w-full mt-4!"
                size="default"
                type="primary"
                :loading="loading"
                :disabled="disabled"
                @click="onLogin(ruleFormRef)"
              >
                登录
              </el-button>
            </Motion>
          </el-form>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
@import url("@/style/login.css");
</style>

<style lang="scss" scoped>
.site-logo {
  object-fit: contain;
}

:deep(.el-input-group__append, .el-input-group__prepend) {
  padding: 0;
}

.captcha-row {
  display: flex;
  gap: 8px;
  width: 100%;
}

.captcha-img {
  flex: 0 0 120px;
  height: 40px;
  cursor: pointer;
  border: 1px solid var(--el-border-color);
  border-radius: 4px;
}
</style>
