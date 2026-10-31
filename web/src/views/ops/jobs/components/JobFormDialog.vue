<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import type { FormInstance, FormRules } from "element-plus";
import { useDebounceFn } from "@vueuse/core";
import { message } from "@/utils/message";
import { isApiError, needsReload } from "@/utils/http/errors";
import { formatDateTime } from "@/utils/format";
import { useFieldErrors } from "@/composables/useFieldErrors";
import {
  createJob,
  previewCron,
  updateJob,
  type Job,
  type JobHandler,
  type JobPayload
} from "@/api/ops/job";
import { CRON_PRESETS, describeCron } from "../../shared/cron";
import { formatSeconds } from "../../shared/format";

defineOptions({ name: "JobFormDialog" });

const props = defineProps<{ job: Job | null; handlers: JobHandler[] }>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{ (e: "saved"): void; (e: "reload"): void }>();

type JobForm = {
  name: string;
  handlerKey: string;
  cronExpr: string;
  paramsText: string;
  enabled: boolean;
  timeoutSeconds: number;
  maxRetries: number;
  remark: string;
};

const PREVIEW_DEBOUNCE_MS = 400;

const emptyForm = (): JobForm => ({
  name: "",
  handlerKey: "",
  cronExpr: "0 0 * * *",
  paramsText: "{}",
  enabled: true,
  timeoutSeconds: 0,
  maxRetries: 0,
  remark: ""
});

const formRef = ref<FormInstance>();
const saving = ref(false);
const form = reactive<JobForm>(emptyForm());
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();
const isEdit = computed(() => props.job !== null);

const selectedHandler = computed(() =>
  props.handlers.find(h => h.key === form.handlerKey)
);
const cronDescription = computed(() => describeCron(form.cronExpr));

/** 解析参数 JSON；返回 null 表示格式错误 */
function parseParams(text: string): Record<string, unknown> | null {
  const trimmed = text.trim();
  if (!trimmed) return {};
  try {
    const value: unknown = JSON.parse(trimmed);
    if (value === null || typeof value !== "object" || Array.isArray(value)) {
      return null;
    }
    return value as Record<string, unknown>;
  } catch {
    return null;
  }
}

const rules = computed<FormRules>(() => ({
  name: [{ required: true, message: "请输入任务名称", trigger: "blur" }],
  handlerKey: [{ required: true, message: "请选择处理器", trigger: "change" }],
  cronExpr: [
    { required: true, message: "请输入 Cron 表达式", trigger: "blur" }
  ],
  paramsText: [
    {
      validator: (_rule, value: string, callback) =>
        parseParams(value) === null
          ? callback(new Error('参数必须是 JSON 对象，如 {"days": 30}'))
          : callback(),
      trigger: "blur"
    }
  ]
}));

// ---- Cron 预览 ----
const preview = reactive({
  loading: false,
  times: [] as string[],
  error: ""
});
let previewSeq = 0;

async function loadPreview(expr: string): Promise<void> {
  const seq = ++previewSeq;
  if (!expr.trim()) {
    preview.times = [];
    preview.error = "";
    return;
  }
  preview.loading = true;
  try {
    const data = await previewCron(expr.trim());
    if (seq !== previewSeq) return;
    preview.times = data.nextTimes;
    preview.error = "";
  } catch (err) {
    if (seq !== previewSeq) return;
    preview.times = [];
    preview.error =
      isApiError(err) && err.fields.length > 0
        ? err.fields[0].message
        : "无法预览执行时间";
  } finally {
    if (seq === previewSeq) preview.loading = false;
  }
}

const debouncedPreview = useDebounceFn(loadPreview, PREVIEW_DEBOUNCE_MS);
watch(
  () => form.cronExpr,
  expr => {
    if (visible.value) debouncedPreview(expr);
  }
);

watch(visible, open => {
  if (!open) return;
  clearFieldErrors();
  const j = props.job;
  Object.assign(
    form,
    j
      ? {
          name: j.name,
          handlerKey: j.handlerKey,
          cronExpr: j.cronExpr,
          paramsText: JSON.stringify(j.params ?? {}, null, 2),
          enabled: j.enabled,
          timeoutSeconds: j.timeoutSeconds,
          maxRetries: j.maxRetries,
          remark: j.remark
        }
      : emptyForm()
  );
  loadPreview(form.cronExpr);
  formRef.value?.clearValidate();
});

function applyPreset(expr: string): void {
  form.cronExpr = expr;
}

function formatParams(): void {
  const parsed = parseParams(form.paramsText);
  if (parsed === null) {
    message("参数不是合法的 JSON 对象", { type: "warning" });
    return;
  }
  form.paramsText = JSON.stringify(parsed, null, 2);
}

function buildPayload(): JobPayload {
  return {
    name: form.name.trim(),
    handlerKey: form.handlerKey,
    cronExpr: form.cronExpr.trim(),
    params: parseParams(form.paramsText) ?? {},
    enabled: form.enabled,
    timeoutSeconds: form.timeoutSeconds ?? 0,
    maxRetries: form.maxRetries ?? 0,
    remark: form.remark
  };
}

async function submit(): Promise<void> {
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;
  clearFieldErrors();
  saving.value = true;
  try {
    const payload = buildPayload();
    if (props.job) {
      await updateJob(props.job.id, { ...payload, version: props.job.version });
    } else {
      await createJob(payload);
    }
    message(isEdit.value ? "任务已更新" : "任务已创建", { type: "success" });
    visible.value = false;
    emit("saved");
  } catch (err) {
    applyFieldErrors(err);
    if (fieldErrors.value.params) {
      fieldErrors.value = {
        ...fieldErrors.value,
        paramsText: fieldErrors.value.params
      };
    }
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
    :title="isEdit ? '编辑定时任务' : '新增定时任务'"
    width="680px"
    destroy-on-close
    :close-on-click-modal="false"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      :validate-on-rule-change="false"
      label-width="96px"
    >
      <el-form-item label="任务名称" prop="name" :error="fieldErrors.name">
        <el-input
          v-model="form.name"
          maxlength="64"
          placeholder="如：清理执行记录"
        />
      </el-form-item>

      <el-form-item
        label="处理器"
        prop="handlerKey"
        :error="fieldErrors.handlerKey"
      >
        <el-select
          v-model="form.handlerKey"
          filterable
          placeholder="选择代码中注册的任务处理器"
          class="w-full"
        >
          <el-option
            v-for="h in handlers"
            :key="h.key"
            :label="h.name"
            :value="h.key"
          >
            <div class="flex items-center justify-between gap-4">
              <span>{{ h.name }}</span>
              <span class="text-xs text-(--el-text-color-secondary) font-mono">
                {{ h.key }}
              </span>
            </div>
          </el-option>
        </el-select>
        <div
          v-if="selectedHandler"
          class="mt-1 text-xs leading-5 text-(--el-text-color-secondary)"
        >
          {{ selectedHandler.description || "暂无说明" }}
        </div>
      </el-form-item>

      <el-form-item
        label="Cron 表达式"
        prop="cronExpr"
        :error="fieldErrors.cronExpr || preview.error"
      >
        <div class="w-full">
          <el-input
            v-model="form.cronExpr"
            maxlength="128"
            class="font-mono"
            placeholder="分 时 日 月 周，如 0 3 * * *；或 @every 10m"
          />
          <div class="mt-2 flex flex-wrap gap-2">
            <el-button
              v-for="p in CRON_PRESETS"
              :key="p.expr"
              size="small"
              :type="form.cronExpr.trim() === p.expr ? 'primary' : 'default'"
              plain
              @click="applyPreset(p.expr)"
            >
              {{ p.label }}
            </el-button>
          </div>
          <div class="job-preview mt-2">
            <div class="flex items-center gap-2 text-sm">
              <span class="text-(--el-text-color-secondary)">含义</span>
              <span class="font-medium">{{ cronDescription || "-" }}</span>
            </div>
            <div
              v-loading="preview.loading"
              class="mt-1 text-xs leading-6 text-(--el-text-color-regular)"
            >
              <template v-if="preview.times.length">
                <div class="text-(--el-text-color-secondary)">
                  接下来 5 次执行
                </div>
                <ol class="list-decimal pl-5 font-mono">
                  <li v-for="t in preview.times" :key="t">
                    {{ formatDateTime(t) }}
                  </li>
                </ol>
              </template>
              <span v-else class="text-(--el-text-color-placeholder)">
                输入有效的表达式后显示接下来的执行时间
              </span>
            </div>
          </div>
        </div>
      </el-form-item>

      <el-form-item
        label="参数"
        prop="paramsText"
        :error="fieldErrors.paramsText"
      >
        <div class="w-full">
          <el-input
            v-model="form.paramsText"
            type="textarea"
            :rows="5"
            class="font-mono"
            placeholder="{}"
          />
          <div class="mt-1 flex items-center justify-between text-xs">
            <span class="text-(--el-text-color-secondary)">
              JSON 对象，传给处理器
            </span>
            <el-button link type="primary" size="small" @click="formatParams">
              格式化
            </el-button>
          </div>
        </div>
      </el-form-item>

      <el-row :gutter="16">
        <el-col :span="12">
          <el-form-item
            label="超时(秒)"
            prop="timeoutSeconds"
            :error="fieldErrors.timeoutSeconds"
          >
            <el-input-number
              v-model="form.timeoutSeconds"
              :min="0"
              :max="86400"
              :step="30"
              controls-position="right"
              class="w-full!"
            />
            <div class="text-xs leading-5 text-(--el-text-color-secondary)">
              0 表示默认<template v-if="selectedHandler">
                （{{ formatSeconds(selectedHandler.defaultTimeoutSeconds) }}）
              </template>
            </div>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item
            label="重试次数"
            prop="maxRetries"
            :error="fieldErrors.maxRetries"
          >
            <el-input-number
              v-model="form.maxRetries"
              :min="0"
              :max="10"
              controls-position="right"
              class="w-full!"
            />
            <div class="text-xs leading-5 text-(--el-text-color-secondary)">
              失败或超时后按指数退避重试
            </div>
          </el-form-item>
        </el-col>
      </el-row>

      <el-form-item label="启用" prop="enabled">
        <el-switch v-model="form.enabled" />
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

<style scoped>
.job-preview {
  padding: 8px 12px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
}
</style>
