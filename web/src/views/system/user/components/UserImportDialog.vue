<script setup lang="ts">
import { computed, ref } from "vue";
import type { UploadFile } from "element-plus";
import {
  downloadImportTemplate,
  importUsers,
  type ImportRowError
} from "@/api/iam/user";
import { isApiError } from "@/utils/http/errors";
import { message } from "@/utils/message";
import { PASSWORD_MAX, PASSWORD_MIN } from "@/views/login/utils/rule";

defineOptions({ name: "UserImportDialog" });

const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{ (e: "imported"): void }>();

const MAX_BYTES = 2 * 1024 * 1024;

const file = ref<File | null>(null);
const password = ref("");
const submitting = ref(false);
const errorText = ref("");
const rowErrors = ref<ImportRowError[]>([]);

const canSubmit = computed(
  () =>
    !!file.value &&
    password.value.length >= PASSWORD_MIN &&
    password.value.length <= PASSWORD_MAX
);

function reset(): void {
  file.value = null;
  password.value = "";
  errorText.value = "";
  rowErrors.value = [];
}

function onFileChange(f: UploadFile): void {
  errorText.value = "";
  rowErrors.value = [];
  if (f.raw && f.raw.size > MAX_BYTES) {
    file.value = null;
    errorText.value = "文件不能超过 2 MB";
    return;
  }
  file.value = f.raw ?? null;
}

async function onTemplate(): Promise<void> {
  try {
    await downloadImportTemplate();
  } catch {
    // 错误提示已由 http 拦截器处理
  }
}

async function submit(): Promise<void> {
  if (!file.value) return;
  submitting.value = true;
  errorText.value = "";
  rowErrors.value = [];
  try {
    const { created } = await importUsers(file.value, password.value);
    message(`已导入 ${created} 个用户，首次登录需修改密码`, {
      type: "success"
    });
    visible.value = false;
    emit("imported");
  } catch (err) {
    if (!isApiError(err)) {
      errorText.value = "导入失败，请稍后重试";
      return;
    }
    errorText.value = err.fields[0]?.message ?? err.message;
    const rows = (err.data as { rows?: ImportRowError[] } | null)?.rows;
    rowErrors.value = Array.isArray(rows) ? rows : [];
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    title="导入用户"
    width="600px"
    :close-on-click-modal="false"
    @closed="reset"
  >
    <ol class="mb-4 pl-4 list-decimal leading-7 text-sm">
      <li>
        <el-button link type="primary" @click="onTemplate">
          下载导入模板
        </el-button>
        ，用 Excel 填写后保存为 CSV（每次最多 1000 行）
      </li>
      <li>
        部门填名称，重名时写路径（如
        总公司/研发部）；角色、岗位填编码或名称，多个用 | 分隔
      </li>
      <li>任一行有误时整批不导入，按下方提示修改后重新上传</li>
    </ol>
    <el-form label-width="90px" @submit.prevent>
      <el-form-item label="CSV 文件" required>
        <el-upload
          :auto-upload="false"
          :show-file-list="false"
          accept=".csv,text/csv"
          :on-change="onFileChange"
        >
          <el-button>选择文件</el-button>
        </el-upload>
        <span v-if="file" class="ml-3 text-sm truncate max-w-[300px]">
          {{ file.name }}
        </span>
      </el-form-item>
      <el-form-item label="初始密码" required>
        <el-input
          v-model="password"
          type="password"
          show-password
          autocomplete="new-password"
          :maxlength="PASSWORD_MAX"
          :placeholder="`${PASSWORD_MIN}–${PASSWORD_MAX} 位，所有导入用户相同，首次登录须修改`"
        />
      </el-form-item>
    </el-form>
    <el-alert
      v-if="errorText"
      :title="errorText"
      type="error"
      :closable="false"
      show-icon
      class="mb-2"
    />
    <el-table
      v-if="rowErrors.length"
      :data="rowErrors"
      size="small"
      max-height="260"
      border
    >
      <el-table-column prop="row" label="行号" width="80" />
      <el-table-column prop="message" label="问题" />
    </el-table>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button
        type="primary"
        :loading="submitting"
        :disabled="!canSubmit"
        @click="submit"
      >
        导入
      </el-button>
    </template>
  </el-dialog>
</template>
