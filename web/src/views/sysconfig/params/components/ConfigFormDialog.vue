<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import type { FormInstance, FormItemRule, FormRules } from "element-plus";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";
import { useFieldErrors } from "@/composables/useFieldErrors";
import {
  CONFIG_VALUE_TYPES,
  createConfig,
  updateConfig,
  type Config,
  type ConfigValueType
} from "@/api/system/config";

defineOptions({ name: "ConfigFormDialog" });

const props = defineProps<{ config: Config | null; groups: string[] }>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{ (e: "saved"): void; (e: "reload"): void }>();

type ConfigForm = {
  key: string;
  name: string;
  groupName: string;
  valueType: ConfigValueType;
  /** string / boolean / json 的值 */
  value: string;
  /** number 类型使用数字输入框 */
  numberValue: number | undefined;
  description: string;
};

const KEY_PATTERN = /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$/;
const VALUE_MAX = 4000;

const emptyForm = (): ConfigForm => ({
  key: "",
  name: "",
  groupName: "default",
  valueType: "string",
  value: "",
  numberValue: 0,
  description: ""
});

const formRef = ref<FormInstance>();
const saving = ref(false);
const form = reactive<ConfigForm>(emptyForm());
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();
const isEdit = computed(() => props.config !== null);

function jsonError(text: string): string {
  try {
    JSON.parse(text);
    return "";
  } catch (err) {
    return err instanceof Error ? err.message : "格式错误";
  }
}

const valueRule: FormItemRule = {
  trigger: "blur",
  validator: (_rule, _value, callback) => {
    if (form.valueType === "number") {
      if (form.numberValue === undefined || form.numberValue === null) {
        callback(new Error("请输入数字"));
        return;
      }
    } else if (form.valueType === "json") {
      if (!form.value.trim()) {
        callback(new Error("请输入 JSON"));
        return;
      }
      const err = jsonError(form.value);
      if (err) {
        callback(new Error(`JSON 格式错误：${err}`));
        return;
      }
    } else if (form.value.length > VALUE_MAX) {
      callback(new Error(`不能多于 ${VALUE_MAX} 个字符`));
      return;
    }
    callback();
  }
};

const rules = computed<FormRules>(() => ({
  key: isEdit.value
    ? []
    : [
        { required: true, message: "请输入参数键", trigger: "blur" },
        {
          pattern: KEY_PATTERN,
          message: "小写字母开头，由小写字母、数字、下划线和点组成",
          trigger: "blur"
        }
      ],
  name: [{ required: true, message: "请输入参数名称", trigger: "blur" }],
  value: [valueRule]
}));

function toNumber(text: string): number | undefined {
  const n = Number(text.trim());
  return text.trim() !== "" && Number.isFinite(n) ? n : undefined;
}

watch(visible, open => {
  if (!open) return;
  clearFieldErrors();
  const c = props.config;
  Object.assign(
    form,
    c
      ? {
          key: c.key,
          name: c.name,
          groupName: c.groupName,
          valueType: c.valueType,
          value: c.value,
          numberValue: c.valueType === "number" ? toNumber(c.value) : 0,
          description: c.description
        }
      : emptyForm()
  );
  formRef.value?.clearValidate();
});

/** 切换类型时尽量保留现有值 */
function onTypeChange(next: ConfigValueType): void {
  if (next === "number") {
    form.numberValue = toNumber(form.value) ?? 0;
  } else if (next === "boolean") {
    form.value = form.value === "true" ? "true" : "false";
  } else if (next === "json" && jsonError(form.value)) {
    form.value = "{}";
  }
  formRef.value?.clearValidate("value");
}

function formatJSON(): void {
  if (jsonError(form.value)) {
    formRef.value?.validateField("value").catch(() => undefined);
    return;
  }
  form.value = JSON.stringify(JSON.parse(form.value), null, 2);
}

function submittedValue(): string {
  return form.valueType === "number"
    ? String(form.numberValue ?? "")
    : form.value;
}

async function submit(): Promise<void> {
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;
  clearFieldErrors();
  saving.value = true;
  const payload = {
    name: form.name,
    groupName: form.groupName,
    valueType: form.valueType,
    value: submittedValue(),
    description: form.description
  };
  try {
    if (props.config) {
      await updateConfig(props.config.id, {
        ...payload,
        version: props.config.version
      });
    } else {
      await createConfig({ ...payload, key: form.key });
    }
    message(isEdit.value ? "参数已更新" : "参数已创建", { type: "success" });
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
    :title="isEdit ? '编辑参数' : '新增参数'"
    width="600px"
    destroy-on-close
    :close-on-click-modal="false"
  >
    <el-alert
      type="warning"
      :closable="false"
      show-icon
      class="mb-4"
      title="参数值以明文保存并记录修改历史，请勿在此填写密码、密钥等敏感信息（应通过环境变量配置）"
    />
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      :validate-on-rule-change="false"
      label-width="90px"
    >
      <el-form-item label="参数名称" prop="name" :error="fieldErrors.name">
        <el-input v-model="form.name" maxlength="64" />
      </el-form-item>
      <el-form-item label="参数键" prop="key" :error="fieldErrors.key">
        <el-input
          v-model="form.key"
          :disabled="isEdit"
          maxlength="128"
          placeholder="如 order.auto_close_minutes"
          class="font-mono"
        />
      </el-form-item>
      <el-form-item
        label="分组"
        prop="groupName"
        :error="fieldErrors.groupName"
      >
        <el-select
          v-model="form.groupName"
          filterable
          allow-create
          default-first-option
          placeholder="选择或输入分组"
          class="w-full"
        >
          <el-option v-for="g in groups" :key="g" :label="g" :value="g" />
        </el-select>
      </el-form-item>
      <el-form-item
        label="值类型"
        prop="valueType"
        :error="fieldErrors.valueType"
      >
        <el-radio-group v-model="form.valueType" @change="onTypeChange">
          <el-radio-button
            v-for="t in CONFIG_VALUE_TYPES"
            :key="t.value"
            :value="t.value"
          >
            {{ t.label }}
          </el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="参数值" prop="value" :error="fieldErrors.value">
        <el-switch
          v-if="form.valueType === 'boolean'"
          v-model="form.value"
          active-value="true"
          inactive-value="false"
          active-text="true"
          inactive-text="false"
          inline-prompt
        />
        <el-input-number
          v-else-if="form.valueType === 'number'"
          v-model="form.numberValue"
          controls-position="right"
          class="w-[240px]!"
        />
        <div v-else-if="form.valueType === 'json'" class="w-full">
          <el-input
            v-model="form.value"
            type="textarea"
            :autosize="{ minRows: 6, maxRows: 16 }"
            :maxlength="VALUE_MAX"
            spellcheck="false"
            class="config-json"
          />
          <div class="mt-1 flex justify-end">
            <el-button link type="primary" size="small" @click="formatJSON">
              格式化
            </el-button>
          </div>
        </div>
        <el-input
          v-else
          v-model="form.value"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 8 }"
          :maxlength="VALUE_MAX"
        />
      </el-form-item>
      <el-form-item
        label="说明"
        prop="description"
        :error="fieldErrors.description"
      >
        <el-input v-model="form.description" type="textarea" maxlength="255" />
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
.config-json :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
}
</style>
