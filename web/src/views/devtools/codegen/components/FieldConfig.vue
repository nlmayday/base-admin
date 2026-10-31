<script setup lang="ts">
import {
  OPTIONS_FROM_COMMENT,
  SEARCH_MODE_LABELS,
  type CodegenDict,
  type CodegenFieldInfo,
  type SearchMode,
  type UploadMode
} from "@/api/devtools/codegen";

defineOptions({ name: "CodegenFieldConfig" });

defineProps<{ loading: boolean; dicts: CodegenDict[] }>();
/** 当前配置；父组件持有默认值，用于计算差异和「恢复默认」 */
const fields = defineModel<CodegenFieldInfo[]>({ required: true });
const emit = defineEmits<{ (e: "reset"): void }>();

const UNSAFE_RE = /["'`\\<>{}]/;

function commentText(f: CodegenFieldInfo): string {
  return f.commentOptions.map(o => `${o.value}=${o.label}`).join(", ");
}

const UPLOAD_LABELS: Record<Exclude<UploadMode, "">, string> = {
  image: "图片上传（存图片地址）",
  files: "附件上传（存附件列表）"
};
/** 「控件」下拉中上传项、关联项的值前缀；其余值为字典编码或注释选项 */
const UPLOAD_PREFIX = "@upload:";
const RELATION_PREFIX = "@relation:";

/** 上传字段不能搜索；使用下拉选项后只能「精确」搜索 */
function modesOf(f: CodegenFieldInfo): SearchMode[] {
  if (f.upload) return ["none"];
  if (f.relation) return f.searchModes.filter(m => m === "none" || m === "eq");
  return f.dict
    ? f.searchModes.filter(m => m === "none" || m === "eq")
    : f.searchModes;
}

function controlOf(f: CodegenFieldInfo): string {
  if (f.relation) return RELATION_PREFIX + f.relation;
  return f.upload ? UPLOAD_PREFIX + f.upload : f.dict;
}

/** 选择控件：上传与下拉选项二选一；下拉选项在列表中显示文字并改为「精确」搜索 */
function setControl(f: CodegenFieldInfo, value: string | undefined): void {
  const v = value ?? "";
  f.relation = "";
  if (v.startsWith(RELATION_PREFIX)) {
    f.relation = v.slice(RELATION_PREFIX.length);
    f.list = true;
    return;
  }
  if (v.startsWith(UPLOAD_PREFIX)) {
    f.upload = v.slice(UPLOAD_PREFIX.length) as UploadMode;
    f.dict = "";
    f.search = "none";
    return;
  }
  f.upload = "";
  f.dict = v;
  if (!f.dict) return;
  f.list = true;
  if (!modesOf(f).includes(f.search)) f.search = "eq";
}

function hasControls(f: CodegenFieldInfo): boolean {
  return (
    f.dictAllowed || f.uploadModes.length > 0 || f.relationLabels.length > 0
  );
}

function labelError(f: CodegenFieldInfo): string {
  const label = f.label.trim();
  if (!label) return "不能为空";
  if (label.length > 20) return "最长 20 个字符";
  if (UNSAFE_RE.test(label)) return "不能包含引号、反斜杠、尖括号或花括号";
  return "";
}
</script>

<template>
  <div>
    <div class="mb-2 flex items-center justify-between">
      <div class="flex items-center gap-2">
        <span class="font-medium">字段配置</span>
        <span class="text-xs text-[var(--el-text-color-secondary)]">
          中文名用于表头、表单和搜索项；不改也可以直接生成
        </span>
      </div>
      <el-button link type="primary" @click="emit('reset')">恢复默认</el-button>
    </div>
    <el-table
      v-loading="loading"
      :data="fields"
      row-key="column"
      border
      size="small"
      max-height="420"
    >
      <el-table-column label="字段" min-width="170">
        <template #default="{ row }">
          <div class="leading-tight">
            <div class="font-mono">{{ row.column }}</div>
            <div class="text-xs text-[var(--el-text-color-secondary)]">
              {{ row.type }}
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="中文名" min-width="200">
        <template #default="{ row }">
          <el-input
            v-model="row.label"
            size="small"
            maxlength="20"
            :class="{ 'is-invalid': labelError(row as CodegenFieldInfo) }"
          />
          <div v-if="labelError(row as CodegenFieldInfo)" class="field-error">
            {{ labelError(row as CodegenFieldInfo) }}
          </div>
          <div
            v-else-if="row.comment && row.comment !== row.label"
            class="truncate text-xs text-[var(--el-text-color-secondary)]"
            :title="row.comment"
          >
            注释：{{ row.comment }}
          </div>
        </template>
      </el-table-column>
      <el-table-column label="列表显示" width="90" align="center">
        <template #default="{ row }">
          <el-checkbox v-model="row.list" />
        </template>
      </el-table-column>
      <el-table-column label="表单" width="90" align="center">
        <template #default="{ row }">
          <el-tooltip
            :disabled="!row.formLocked"
            :content="row.formHint"
            placement="top"
          >
            <el-checkbox v-model="row.form" :disabled="row.formLocked" />
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column label="控件" width="210">
        <template #default="{ row }">
          <el-select
            v-if="hasControls(row as CodegenFieldInfo)"
            :model-value="controlOf(row as CodegenFieldInfo)"
            size="small"
            clearable
            filterable
            placeholder="默认"
            @update:model-value="
              (v: string | undefined) => setControl(row as CodegenFieldInfo, v)
            "
          >
            <el-option-group
              v-if="row.relationLabels.length"
              :label="`关联 ${row.relationTable}`"
            >
              <el-option
                v-for="col in row.relationLabels"
                :key="col"
                :label="`关联：显示 ${col}`"
                :value="RELATION_PREFIX + col"
              />
            </el-option-group>
            <el-option-group v-if="row.uploadModes.length" label="上传">
              <el-option
                v-for="m in row.uploadModes"
                :key="m"
                :label="UPLOAD_LABELS[m as Exclude<UploadMode, ''>]"
                :value="UPLOAD_PREFIX + m"
              />
            </el-option-group>
            <el-option-group
              v-if="row.dictAllowed && row.commentOptions.length"
              label="字段注释中的选项"
            >
              <el-option
                :label="`注释：${commentText(row as CodegenFieldInfo)}`"
                :value="OPTIONS_FROM_COMMENT"
              />
            </el-option-group>
            <el-option-group v-if="row.dictAllowed" label="数据字典">
              <el-option
                v-for="d in dicts"
                :key="d.code"
                :label="`${d.name}（${d.code}）`"
                :value="d.code"
                :disabled="!d.enabled"
              />
            </el-option-group>
          </el-select>
          <span v-else class="text-xs text-[var(--el-text-color-placeholder)]">
            —
          </span>
        </template>
      </el-table-column>
      <el-table-column label="搜索方式" width="140">
        <template #default="{ row }">
          <el-select
            v-model="row.search"
            size="small"
            :disabled="modesOf(row as CodegenFieldInfo).length < 2"
          >
            <el-option
              v-for="m in modesOf(row as CodegenFieldInfo)"
              :key="m"
              :label="SEARCH_MODE_LABELS[m]"
              :value="m"
            />
          </el-select>
        </template>
      </el-table-column>
    </el-table>
    <div class="mt-2 text-xs leading-5 text-[var(--el-text-color-secondary)]">
      搜索方式：模糊＝包含输入的文字；精确＝等于；范围＝在两个值之间（数字、金额填最小/最大值，日期、时间选起止日期）。每个搜索字段在列表页生成一个独立的搜索项。
      控件：关联——字段是外键（如 customer_id
      指向客户表）时，表单为可搜索的下拉，列表显示所选记录的名称，可选择显示哪一列；上传——图片字段（文本，长度
      ≥ 255）用图片上传，附件字段（jsonb）用附件上传，按字段名或注释（如
      cover_url、封面、attachments、附件）自动选择；下拉选项——表单与搜索变为下拉框，列表显示选项文字，保存时校验取值，选项来自「数据字典」（运行时可维护）或字段注释（写成「类型：1=通知,2=公告」）。不选则为默认输入框。
    </div>
  </div>
</template>

<style scoped>
.field-error {
  font-size: 12px;
  line-height: 1.4;
  color: var(--el-color-danger);
}

.is-invalid :deep(.el-input__wrapper) {
  box-shadow: 0 0 0 1px var(--el-color-danger) inset;
}
</style>
