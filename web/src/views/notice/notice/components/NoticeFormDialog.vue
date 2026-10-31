<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import type { FormInstance, FormRules } from "element-plus";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";
import { useFieldErrors } from "@/composables/useFieldErrors";
import { useDict } from "@/composables/useDict";
import {
  createNotice,
  updateNotice,
  type Notice,
  type NoticePayload
} from "@/api/notice/notice";
import type { AttachmentRef } from "@/api/file";
import ReFileUpload from "@/components/ReUpload/FileUpload.vue";

defineOptions({ name: "NoticeFormDialog" });

const props = defineProps<{ row: Notice | null }>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{ (e: "saved"): void; (e: "reload"): void }>();

type NoticeForm = {
  title: string;
  type: number;
  content: string;
  attachments: AttachmentRef[];
};

const emptyForm = (): NoticeForm => ({
  title: "",
  type: 1,
  content: "",
  attachments: []
});

const formRef = ref<FormInstance>();
const saving = ref(false);
const form = reactive<NoticeForm>(emptyForm());
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();
const isEdit = computed(() => props.row !== null);
const { items: typeOptions } = useDict("notice_type");

const rules: FormRules = {
  title: [{ required: true, message: "请输入标题", trigger: "blur" }],
  content: [{ required: true, message: "请输入内容", trigger: "blur" }]
};

watch(visible, open => {
  if (!open) return;
  clearFieldErrors();
  const r = props.row;
  Object.assign(
    form,
    r
      ? {
          title: r.title,
          type: r.type,
          content: r.content,
          attachments: [...r.attachments]
        }
      : emptyForm()
  );
  formRef.value?.clearValidate();
});

function toPayload(): NoticePayload {
  return {
    title: form.title,
    type: form.type,
    content: form.content,
    attachments: form.attachments
  };
}

async function submit(): Promise<void> {
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;
  clearFieldErrors();
  saving.value = true;
  try {
    if (props.row) {
      await updateNotice(props.row.id, {
        ...toPayload(),
        version: props.row.version
      });
    } else {
      await createNotice(toPayload());
    }
    message(isEdit.value ? "通知公告已更新" : "通知公告已创建", {
      type: "success"
    });
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
    :title="isEdit ? '编辑通知公告' : '新增通知公告'"
    width="680px"
    destroy-on-close
    :close-on-click-modal="false"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      :validate-on-rule-change="false"
      label-width="100px"
    >
      <el-form-item label="标题" prop="title" :error="fieldErrors.title">
        <el-input v-model="form.title" maxlength="128" />
      </el-form-item>
      <el-form-item label="类型" prop="type" :error="fieldErrors.type">
        <el-select
          v-model="form.type"
          placeholder="请选择类型"
          clearable
          class="w-full"
        >
          <el-option
            v-for="o in typeOptions"
            :key="o.value"
            :label="o.label"
            :value="Number(o.value)"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="内容" prop="content" :error="fieldErrors.content">
        <el-input
          v-model="form.content"
          type="textarea"
          :rows="10"
          maxlength="10000"
          show-word-limit
          placeholder="纯文本，换行会原样显示"
        />
      </el-form-item>
      <el-form-item
        label="附件"
        prop="attachments"
        :error="fieldErrors.attachments"
      >
        <ReFileUpload v-model="form.attachments" :limit="10" />
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
