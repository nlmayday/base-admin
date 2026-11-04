<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import type { FormInstance, FormItemRule, FormRules } from "element-plus";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";
import { useFieldErrors } from "@/composables/useFieldErrors";
import { hasPerm } from "@/utils/permission";
import { useSettingStoreHook } from "@/store/modules/settings";
import { PAGE_SIZES, setDefaultPageSize } from "@/utils/pageSize";
import {
  getSettings,
  updateSettings,
  type Settings,
  type SettingsView
} from "@/api/system/settings";

defineOptions({ name: "SystemSettings" });

// 模板属性里写 "//" 会让 vue-tsc（3.3）误判后续 v-model 的类型，放在脚本里
const LOGO_PLACEHOLDER = "https://… 或 /static/logo.png，留空使用默认 Logo";

/** 与后端一致：这些扩展名永远不允许上传 */
const DENIED_EXTENSIONS = new Set([
  "html",
  "htm",
  "xhtml",
  "shtml",
  "svg",
  "svgz",
  "xml",
  "js",
  "mjs",
  "cjs",
  "exe",
  "msi",
  "dll",
  "com",
  "scr",
  "bat",
  "cmd",
  "sh",
  "ps1",
  "vbs",
  "jar",
  "hta",
  "php",
  "jsp",
  "asp",
  "aspx"
]);
const EXTENSION_PATTERN = /^[a-z0-9]{1,16}$/;
const MAX_EXTENSIONS = 100;
const SIZE_MIN = 1;
const SIZE_MAX = 1024;
const RETENTION_MIN = 7;
const RETENTION_MAX = 3650;
/** 与后端 core/export 一致 */
const EXPORT_MIN = 1000;
const EXPORT_MAX = 1000000;

const canUpdate = computed(() => hasPerm("system:settings:update"));

const formRef = ref<FormInstance>();
const loading = ref(false);
const saving = ref(false);
const loaded = ref<SettingsView | null>(null);
const form = reactive<Settings>({
  siteName: "",
  logoUrl: "",
  uploadAllowedExtensions: [],
  uploadMaxSizeMB: 20,
  auditRetentionDays: 180,
  exportMaxRows: 100000,
  pageSize: 20
});
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();
const logoBroken = ref(false);

function validLogoUrl(v: string): boolean {
  if (!v) return true;
  if (/[\s\\]/.test(v)) return false;
  if (v.startsWith("/")) return !v.startsWith("//");
  try {
    const u = new URL(v);
    return (u.protocol === "http:" || u.protocol === "https:") && !!u.host;
  } catch {
    return false;
  }
}

const logoPreview = computed(() =>
  form.logoUrl && validLogoUrl(form.logoUrl) ? form.logoUrl : ""
);

const extensionsRule: FormItemRule = {
  trigger: "change",
  validator: (_rule, value: string[], callback) => {
    if (!value || value.length === 0) {
      callback(new Error("至少需要一个扩展名"));
      return;
    }
    if (value.length > MAX_EXTENSIONS) {
      callback(new Error(`不能多于 ${MAX_EXTENSIONS} 项`));
      return;
    }
    const bad = value.find(e => !EXTENSION_PATTERN.test(e));
    if (bad) {
      callback(new Error(`扩展名「${bad}」无效，只能包含小写字母和数字`));
      return;
    }
    const denied = value.find(e => DENIED_EXTENSIONS.has(e));
    if (denied) {
      callback(new Error(`出于安全考虑，不允许上传 .${denied} 文件`));
      return;
    }
    callback();
  }
};

const rules: FormRules = {
  siteName: [
    { required: true, message: "请输入站点名称", trigger: "blur" },
    { max: 64, message: "不能多于 64 个字符", trigger: "blur" }
  ],
  logoUrl: [
    {
      trigger: "blur",
      validator: (_rule, value: string, callback) =>
        validLogoUrl(value?.trim() ?? "")
          ? callback()
          : callback(new Error("须为 http(s) 地址或以 / 开头的站内路径"))
    }
  ],
  uploadAllowedExtensions: [extensionsRule]
};

function fill(view: SettingsView): void {
  loaded.value = view;
  Object.assign(form, {
    siteName: view.siteName,
    logoUrl: view.logoUrl,
    uploadAllowedExtensions: [...view.uploadAllowedExtensions],
    uploadMaxSizeMB: view.uploadMaxSizeMB,
    auditRetentionDays: view.auditRetentionDays,
    exportMaxRows: view.exportMaxRows,
    pageSize: view.pageSize
  });
  logoBroken.value = false;
  clearFieldErrors();
  formRef.value?.clearValidate();
}

async function load(): Promise<void> {
  loading.value = true;
  try {
    const view = await getSettings();
    if (view) fill(view);
  } catch {
    // 错误提示已由 http 拦截器统一处理
  } finally {
    loading.value = false;
  }
}

/** 标签输入：统一小写、去掉前导点、去重 */
function normalizeExtensions(): void {
  const seen = new Set<string>();
  form.uploadAllowedExtensions = form.uploadAllowedExtensions
    .map(e => e.trim().replace(/^\.+/, "").toLowerCase())
    .filter(e => e !== "" && !seen.has(e) && (seen.add(e), true));
  formRef.value?.validateField("uploadAllowedExtensions").catch(() => {});
}

async function submit(): Promise<void> {
  if (!loaded.value) return;
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;
  clearFieldErrors();
  saving.value = true;
  try {
    const view = await updateSettings({
      siteName: form.siteName.trim(),
      logoUrl: form.logoUrl.trim(),
      uploadAllowedExtensions: form.uploadAllowedExtensions,
      uploadMaxSizeMB: form.uploadMaxSizeMB,
      auditRetentionDays: form.auditRetentionDays,
      exportMaxRows: form.exportMaxRows,
      pageSize: form.pageSize,
      version: loaded.value.version
    });
    if (view) {
      fill(view);
      useSettingStoreHook().applyBranding(view);
      setDefaultPageSize(view.pageSize);
    }
    message("设置已保存", { type: "success" });
  } catch (err) {
    applyFieldErrors(err);
    if (needsReload(err)) load();
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div v-loading="loading" class="main">
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      :validate-on-rule-change="false"
      :disabled="!canUpdate"
      label-width="140px"
      class="settings-form"
    >
      <el-card shadow="never" class="mb-3">
        <template #header>
          <div class="font-medium">站点设置</div>
          <div class="settings-hint">
            站点名称和 Logo 会显示在登录页，无需登录即可读取
          </div>
        </template>
        <el-form-item
          label="站点名称"
          prop="siteName"
          :error="fieldErrors.siteName"
        >
          <el-input
            v-model="form.siteName"
            maxlength="64"
            show-word-limit
            class="max-w-[420px]"
          />
        </el-form-item>
        <el-form-item
          label="站点 Logo"
          prop="logoUrl"
          :error="fieldErrors.logoUrl"
        >
          <div class="flex w-full max-w-[640px] items-start gap-3">
            <el-input
              v-model="form.logoUrl"
              maxlength="512"
              clearable
              :placeholder="LOGO_PLACEHOLDER"
              @input="logoBroken = false"
            />
            <div class="logo-preview">
              <img
                v-if="logoPreview && !logoBroken"
                :src="logoPreview"
                alt="Logo 预览"
                referrerpolicy="no-referrer"
                @error="logoBroken = true"
              />
              <span v-else class="text-xs">
                {{ logoBroken ? "无法加载" : "默认" }}
              </span>
            </div>
          </div>
        </el-form-item>
      </el-card>

      <el-card shadow="never" class="mb-3">
        <template #header>
          <div class="font-medium">上传设置</div>
          <div class="settings-hint">
            文件上传按此处的扩展名白名单和大小限制校验；HTML、SVG、脚本和可执行文件始终禁止
          </div>
        </template>
        <el-form-item
          label="允许的扩展名"
          prop="uploadAllowedExtensions"
          :error="fieldErrors.uploadAllowedExtensions"
        >
          <el-input-tag
            v-model="form.uploadAllowedExtensions"
            placeholder="输入扩展名后按回车，如 png"
            :max="MAX_EXTENSIONS"
            tag-type="primary"
            clearable
            class="max-w-[640px]"
            @add-tag="normalizeExtensions"
            @remove-tag="normalizeExtensions"
          />
        </el-form-item>
        <el-form-item
          label="单文件大小上限"
          prop="uploadMaxSizeMB"
          :error="fieldErrors.uploadMaxSizeMB"
        >
          <el-input-number
            v-model="form.uploadMaxSizeMB"
            :min="SIZE_MIN"
            :max="SIZE_MAX"
            :step="1"
            step-strictly
            controls-position="right"
          />
          <span class="ml-2 text-[var(--el-text-color-secondary)]">MB</span>
        </el-form-item>
      </el-card>

      <el-card shadow="never" class="mb-3">
        <template #header>
          <div class="font-medium">日志设置</div>
          <div class="settings-hint">
            超过保留天数的登录日志和操作日志会被定时清理
          </div>
        </template>
        <el-form-item
          label="日志保留天数"
          prop="auditRetentionDays"
          :error="fieldErrors.auditRetentionDays"
        >
          <el-input-number
            v-model="form.auditRetentionDays"
            :min="RETENTION_MIN"
            :max="RETENTION_MAX"
            :step="1"
            step-strictly
            controls-position="right"
          />
          <span class="ml-2 text-[var(--el-text-color-secondary)]">天</span>
        </el-form-item>
      </el-card>

      <el-card shadow="never" class="mb-3">
        <template #header>
          <div class="font-medium">导出与列表</div>
          <div class="settings-hint">
            导出超过上限时直接拒绝并提示缩小查询范围，不会截断；导出中的时间为
            UTC，表头标注 (UTC)。每页条数对之后打开的列表生效
          </div>
        </template>
        <el-form-item
          label="导出行数上限"
          prop="exportMaxRows"
          :error="fieldErrors.exportMaxRows"
        >
          <el-input-number
            v-model="form.exportMaxRows"
            :min="EXPORT_MIN"
            :max="EXPORT_MAX"
            :step="1000"
            controls-position="right"
            class="w-[200px]!"
          />
          <span class="ml-2 text-[var(--el-text-color-secondary)]">行</span>
        </el-form-item>
        <el-form-item
          label="列表默认每页条数"
          prop="pageSize"
          :error="fieldErrors.pageSize"
        >
          <el-select v-model="form.pageSize" class="w-[200px]!">
            <el-option
              v-for="n in PAGE_SIZES"
              :key="n"
              :label="`${n} 条/页`"
              :value="n"
            />
          </el-select>
        </el-form-item>
      </el-card>
    </el-form>

    <el-card shadow="never" body-class="py-3!">
      <div class="flex items-center justify-end gap-2">
        <span
          v-if="!canUpdate"
          class="mr-auto text-sm text-[var(--el-text-color-secondary)]"
        >
          你没有修改权限，当前为只读模式
        </span>
        <el-button :disabled="loading || saving" @click="load">重置</el-button>
        <el-button
          v-perm="'system:settings:update'"
          type="primary"
          :loading="saving"
          :disabled="loading || !loaded"
          data-testid="settings-save"
          @click="submit"
        >
          保存设置
        </el-button>
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.settings-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.logo-preview {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: 64px;
  height: 32px;
  overflow: hidden;
  color: var(--el-text-color-placeholder);
  background: var(--el-fill-color-light);
  border: 1px dashed var(--el-border-color);
  border-radius: 4px;
}

.logo-preview img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}
</style>
