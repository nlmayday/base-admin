<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { message } from "@/utils/message";
import { hasPerm } from "@/utils/permission";
import { useFieldErrors } from "@/composables/useFieldErrors";
import {
  getPolicy,
  unlockAccount,
  updatePolicy,
  type CaptchaMode,
  type SecurityPolicy,
  type SecurityPolicyView
} from "@/api/security";

defineOptions({ name: "AuthPolicy" });

const CAPTCHA_MODES: { value: CaptchaMode; label: string; desc: string }[] = [
  { value: "off", label: "关闭", desc: "登录时不使用验证码" },
  {
    value: "after_failures",
    label: "失败后启用",
    desc: "同一账号连续失败达到次数后需要验证码"
  },
  { value: "always", label: "始终启用", desc: "每次登录都需要输入验证码" }
];

const loading = ref(false);
const saving = ref(false);
const unlocking = ref(false);
const unlockName = ref("");
const view = ref<SecurityPolicyView | null>(null);
const form = reactive<SecurityPolicy>({
  passwordMinLength: 8,
  passwordRequireUpper: false,
  passwordRequireLower: false,
  passwordRequireDigit: true,
  passwordRequireSymbol: false,
  captchaMode: "after_failures",
  captchaAfterFailures: 3,
  lockoutThreshold: 5,
  lockoutMinutes: 15,
  version: 0
});
const { fieldErrors, applyFieldErrors, clearFieldErrors } = useFieldErrors();
const editable = computed(() => hasPerm("auth:policy:update"));

/** 当前规则的文字描述，便于管理员确认效果 */
const passwordRuleText = computed(() => {
  const classes = [
    form.passwordRequireUpper && "大写字母",
    form.passwordRequireLower && "小写字母",
    form.passwordRequireDigit && "数字",
    form.passwordRequireSymbol && "特殊字符"
  ].filter(Boolean);
  const base = `至少 ${form.passwordMinLength} 位`;
  return classes.length ? `${base}，且须包含${classes.join("、")}` : base;
});

async function load(): Promise<void> {
  loading.value = true;
  try {
    const p = await getPolicy();
    view.value = p;
    Object.assign(form, {
      passwordMinLength: p.passwordMinLength,
      passwordRequireUpper: p.passwordRequireUpper,
      passwordRequireLower: p.passwordRequireLower,
      passwordRequireDigit: p.passwordRequireDigit,
      passwordRequireSymbol: p.passwordRequireSymbol,
      captchaMode: p.captchaMode,
      captchaAfterFailures: p.captchaAfterFailures,
      lockoutThreshold: p.lockoutThreshold,
      lockoutMinutes: p.lockoutMinutes,
      version: p.version
    });
  } finally {
    loading.value = false;
  }
}

async function save(): Promise<void> {
  saving.value = true;
  clearFieldErrors();
  try {
    view.value = await updatePolicy({ ...form });
    form.version = view.value.version;
    message("安全策略已保存", { type: "success" });
  } catch (err) {
    applyFieldErrors(err);
  } finally {
    saving.value = false;
  }
}

async function unlock(): Promise<void> {
  const name = unlockName.value.trim();
  if (!name) return;
  unlocking.value = true;
  try {
    await unlockAccount(name);
    message(`已解除「${name}」的登录锁定`, { type: "success" });
    unlockName.value = "";
  } finally {
    unlocking.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div v-loading="loading" class="main">
    <el-row :gutter="16">
      <el-col :xs="24" :lg="12">
        <el-card shadow="never" class="mb-4">
          <template #header>
            <div class="card-title">
              <span>密码规则</span>
              <span class="card-sub">新建用户、重置密码、修改密码时生效</span>
            </div>
          </template>
          <el-form label-width="110px" :disabled="!editable">
            <el-form-item
              label="最小长度"
              :error="fieldErrors.passwordMinLength"
            >
              <el-input-number
                v-model="form.passwordMinLength"
                :min="8"
                :max="72"
              />
              <span class="hint">位（8–72）</span>
            </el-form-item>
            <el-form-item label="必须包含">
              <el-checkbox v-model="form.passwordRequireUpper">
                大写字母
              </el-checkbox>
              <el-checkbox v-model="form.passwordRequireLower">
                小写字母
              </el-checkbox>
              <el-checkbox v-model="form.passwordRequireDigit">
                数字
              </el-checkbox>
              <el-checkbox v-model="form.passwordRequireSymbol">
                特殊字符
              </el-checkbox>
            </el-form-item>
            <el-form-item label="规则预览">
              <el-tag type="info" effect="plain">{{ passwordRuleText }}</el-tag>
            </el-form-item>
          </el-form>
        </el-card>

        <el-card shadow="never" class="mb-4">
          <template #header>
            <div class="card-title">
              <span>会话</span>
              <span class="card-sub">由服务端配置文件决定，此处只读</span>
            </div>
          </template>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="空闲超时">
              {{ view?.sessionIdleMinutes ?? "-" }} 分钟无操作后失效
            </el-descriptions-item>
            <el-descriptions-item label="最长有效期">
              {{ view?.sessionAbsoluteMinutes ?? "-" }} 分钟后必须重新登录
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>

      <el-col :xs="24" :lg="12">
        <el-card shadow="never" class="mb-4">
          <template #header>
            <div class="card-title">
              <span>登录保护</span>
              <span class="card-sub"
                >按账号统计连续失败次数，登录成功后清零</span
              >
            </div>
          </template>
          <el-form label-width="110px" :disabled="!editable">
            <el-form-item label="图形验证码">
              <div class="mode-list">
                <div
                  v-for="m in CAPTCHA_MODES"
                  :key="m.value"
                  class="mode-card"
                  :class="{ active: form.captchaMode === m.value }"
                  @click="editable && (form.captchaMode = m.value)"
                >
                  <el-radio
                    :model-value="form.captchaMode"
                    :value="m.value"
                    @change="form.captchaMode = m.value"
                  >
                    {{ m.label }}
                  </el-radio>
                  <div class="mode-desc">{{ m.desc }}</div>
                </div>
              </div>
            </el-form-item>
            <el-form-item
              v-if="form.captchaMode === 'after_failures'"
              label="触发次数"
            >
              失败
              <el-input-number
                v-model="form.captchaAfterFailures"
                :min="1"
                :max="20"
                class="mx-2"
              />
              次后需要验证码
            </el-form-item>
            <el-form-item label="账号锁定">
              连续失败
              <el-input-number
                v-model="form.lockoutThreshold"
                :min="0"
                :max="100"
                class="mx-2"
              />
              次，锁定
              <el-input-number
                v-model="form.lockoutMinutes"
                :min="1"
                :max="1440"
                class="mx-2"
              />
              分钟
            </el-form-item>
            <el-form-item>
              <el-alert
                v-if="form.lockoutThreshold === 0"
                type="warning"
                :closable="false"
                show-icon
                title="次数为 0 表示不锁定账号，仅依赖登录限流与验证码"
              />
              <el-alert
                v-else
                type="info"
                :closable="false"
                show-icon
                title="锁定按账号生效，攻击者也可借此锁住他人账号；如遇误锁可在下方手动解除"
              />
            </el-form-item>
          </el-form>
        </el-card>

        <el-card v-if="editable" shadow="never" class="mb-4">
          <template #header>
            <div class="card-title">
              <span>解除锁定</span>
              <span class="card-sub">提前解除某个账号的登录锁定</span>
            </div>
          </template>
          <div class="flex gap-2">
            <el-input
              v-model="unlockName"
              placeholder="输入用户名"
              clearable
              @keyup.enter="unlock"
            />
            <el-button
              type="primary"
              plain
              :loading="unlocking"
              :disabled="!unlockName.trim()"
              @click="unlock"
            >
              解除锁定
            </el-button>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <div v-if="editable" class="footer-bar">
      <el-button @click="load">重置</el-button>
      <el-button type="primary" :loading="saving" @click="save">
        保存策略
      </el-button>
    </div>
  </div>
</template>

<style scoped>
.card-title {
  display: flex;
  gap: 12px;
  align-items: baseline;
  font-weight: 500;
}

.card-sub,
.hint {
  font-size: 12px;
  font-weight: normal;
  color: var(--el-text-color-secondary);
}

.hint {
  margin-left: 8px;
}

.mode-list {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  width: 100%;
}

.mode-card {
  padding: 8px 12px;
  cursor: pointer;
  border: 1px solid var(--el-border-color);
  border-radius: 6px;
  transition: border-color 0.2s;
}

.mode-card.active {
  background: var(--el-color-primary-light-9);
  border-color: var(--el-color-primary);
}

.mode-desc {
  font-size: 12px;
  line-height: 1.4;
  color: var(--el-text-color-secondary);
}

.footer-bar {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  padding: 12px 16px;
  background: var(--el-bg-color);
  border-radius: 4px;
}
</style>
