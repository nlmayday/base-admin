<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useClipboard } from "@vueuse/core";
import type { FormInstance, FormRules } from "element-plus";
import { message } from "@/utils/message";
import { useFieldErrors } from "@/composables/useFieldErrors";
import { createAgentKey } from "@/api/devtools/agent";
import { agentPrompt } from "../prompt";

defineOptions({ name: "CreateAgentKeyDialog" });

const props = defineProps<{ baseUrl: string }>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{ (e: "created"): void }>();

const EXPIRY_OPTIONS = [
  { label: "1 天", value: 1 },
  { label: "7 天", value: 7 },
  { label: "30 天", value: 30 }
];

const formRef = ref<FormInstance>();
const saving = ref(false);
const form = reactive({ name: "", expiresInDays: 7 });
/** 创建成功后的完整密钥，只显示这一次 */
const created = ref("");
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();
const { copy } = useClipboard({ legacy: true });

const rules: FormRules = {
  name: [{ required: true, message: "请输入用途", trigger: "blur" }]
};
const prompt = computed(() => agentPrompt(props.baseUrl, created.value));

watch(visible, open => {
  if (!open) return;
  Object.assign(form, { name: "", expiresInDays: 7 });
  created.value = "";
  clearFieldErrors();
  formRef.value?.clearValidate();
});

async function submit(): Promise<void> {
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;
  clearFieldErrors();
  saving.value = true;
  try {
    const res = await createAgentKey({ ...form, name: form.name.trim() });
    created.value = res.key;
    emit("created");
  } catch (err) {
    applyFieldErrors(err);
  } finally {
    saving.value = false;
  }
}

async function copyText(text: string, done: string): Promise<void> {
  try {
    await copy(text);
    message(done, { type: "success" });
  } catch {
    message("复制失败，请手动选择文本复制", { type: "error" });
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="created ? '密钥已创建' : '创建密钥'"
    :width="created ? '760px' : '540px'"
    destroy-on-close
    :close-on-click-modal="false"
  >
    <template v-if="created">
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        class="mb-3"
        title="密钥只显示这一次，关闭后无法再查看，请立即复制保存。"
      />
      <div class="mb-1 text-sm text-[var(--el-text-color-secondary)]">密钥</div>
      <el-input :model-value="created" readonly class="mb-4">
        <template #append>
          <el-button @click="copyText(created, '密钥已复制')">复制</el-button>
        </template>
      </el-input>
      <div class="mb-1 flex items-center justify-between">
        <span class="text-sm text-[var(--el-text-color-secondary)]">
          把这段说明发给 AI（Claude Code、Cursor
          等），它会先读取接口说明，再按约定建表、生成代码
        </span>
        <el-button
          type="primary"
          link
          @click="copyText(prompt, '说明已复制，可直接发给 AI')"
        >
          复制说明
        </el-button>
      </div>
      <el-input
        :model-value="prompt"
        type="textarea"
        readonly
        :autosize="{ minRows: 6, maxRows: 12 }"
        class="agent-prompt"
      />
    </template>
    <el-form
      v-else
      ref="formRef"
      :model="form"
      :rules="rules"
      label-width="80px"
      @submit.prevent
    >
      <el-form-item label="用途" prop="name" :error="fieldErrors.name">
        <el-input
          v-model="form.name"
          maxlength="64"
          placeholder="如：Claude Code 本机"
        />
      </el-form-item>
      <el-form-item
        label="有效期"
        prop="expiresInDays"
        :error="fieldErrors.expiresInDays"
      >
        <el-radio-group v-model="form.expiresInDays">
          <el-radio-button
            v-for="o in EXPIRY_OPTIONS"
            :key="o.value"
            :value="o.value"
          >
            {{ o.label }}
          </el-radio-button>
        </el-radio-group>
      </el-form-item>
      <div class="pl-[80px] text-xs text-[var(--el-text-color-secondary)]">
        密钥以你的身份调用，需要「使用代码生成」权限；只在开发环境可用。任务做完后请撤销。
      </div>
    </el-form>
    <template #footer>
      <template v-if="created">
        <el-button type="primary" @click="visible = false">完成</el-button>
      </template>
      <template v-else>
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">
          创建
        </el-button>
      </template>
    </template>
  </el-dialog>
</template>

<style scoped>
.agent-prompt :deep(textarea) {
  font-family: var(--el-font-family-mono, ui-monospace, monospace);
  font-size: 12px;
  line-height: 1.7;
}
</style>
