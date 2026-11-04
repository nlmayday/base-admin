<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { ElMessageBox, type FormInstance, type FormRules } from "element-plus";
import { useClipboard } from "@vueuse/core";
import { message } from "@/utils/message";
import { isApiError } from "@/utils/http/errors";
import { useFieldErrors } from "@/composables/useFieldErrors";
import {
  CODEGEN_DISABLED,
  CODEGEN_EXISTS,
  generateCodegen,
  getCodegenFields,
  getLastCodegen,
  listCodegenDicts,
  listCodegenTables,
  previewCodegen,
  type CodegenDict,
  type CodegenFieldInfo,
  type CodegenFieldOption,
  type CodegenGenerateResult,
  type CodegenPreview,
  type CodegenRequest,
  type CodegenTable
} from "@/api/devtools/codegen";
import CodeBlock from "../shared/CodeBlock.vue";
import PreviewPanel from "./components/PreviewPanel.vue";
import FieldConfig from "./components/FieldConfig.vue";
import AdvancedOptions, {
  type AdvancedModel
} from "./components/AdvancedOptions.vue";

defineOptions({ name: "DevtoolsCodegen" });

const MODULE_RE = /^[a-z][a-z0-9]{1,30}$/;
const RESOURCE_RE = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/;
const UNSAFE_RE = /["'`\\<>{}]/;

const CLI_USAGE = `cd server
go run ./cmd/gen -table biz_orders -module sales -resource order -name '订单' \\
  [-module-title '销售管理'] [-fields '<字段配置 JSON>'] [-key-mode generated] \\
  [-read-only] [-children '<子表 JSON>'] [-force] [-dry-run]
# 批量：每项与本页的请求相同（table、module、resource、name、fields、keyMode、readOnly、children）
go run ./cmd/gen -spec specs.json [-force]`;

const loadingTables = ref(false);
const disabled = ref(false);
const loadError = ref("");
const tables = ref<CodegenTable[]>([]);
const previewing = ref(false);
const preview = ref<CodegenPreview | null>(null);
const formRef = ref<FormInstance>();
const form = reactive<CodegenRequest>({
  table: "",
  module: "",
  resource: "",
  name: "",
  moduleTitle: ""
});
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();
const emptyAdvanced = (): AdvancedModel => ({
  keyMode: "",
  readOnly: false,
  refs: [],
  children: []
});
const advanced = ref<AdvancedModel>(emptyAdvanced());
/** 字段配置：defaults 是后端给出的默认值，fields 是当前编辑的副本 */
const defaults = ref<CodegenFieldInfo[]>([]);
const fields = ref<CodegenFieldInfo[]>([]);
const dicts = ref<CodegenDict[]>([]);
const loadingFields = ref(false);
const generating = ref(false);
const overwrite = ref(false);
const result = ref<CodegenGenerateResult | null>(null);
const restoring = ref(false);
const wiredText = computed(() => {
  if (!result.value?.wired) return "";
  return result.value.newModule
    ? "，新模块已接入 server/internal/app/modules.go"
    : "，新资源已注册到所属模块的 module.go";
});

/**
 * 写入新的页面文件后，Vite 开发服务器会刷新整个页面（路由按文件动态加载），
 * 响应往往来不及显示。生成前留下标记，刷新后据此取回结果。
 */
const PENDING_KEY = "codegen:pending";

function markPending(on: boolean): void {
  try {
    if (on) sessionStorage.setItem(PENDING_KEY, String(Date.now()));
    else sessionStorage.removeItem(PENDING_KEY);
  } catch {
    // 无法使用 sessionStorage 时只是不能在刷新后恢复结果
  }
}

/** 生成开始的时间；没有未完成的标记时为 0 */
function takePending(): number {
  try {
    const at = Number(sessionStorage.getItem(PENDING_KEY));
    sessionStorage.removeItem(PENDING_KEY);
    return at > 0 && Date.now() - at < 5 * 60_000 ? at : 0;
  } catch {
    return 0;
  }
}

/** 等待后端完成本次生成（刷新可能早于响应），最多约 10 秒 */
async function waitForLast(since: number) {
  for (let i = 0; i < 20; i++) {
    const last = await getLastCodegen().catch(() => null);
    if (last && Date.parse(last.at) >= since - 1000) return last;
    await new Promise(resolve => setTimeout(resolve, 500));
  }
  return null;
}

/** 刷新后恢复上一次生成的表单、字段配置与结果 */
async function restoreLast(): Promise<void> {
  const since = takePending();
  if (!since) return;
  restoring.value = true;
  const last = await waitForLast(since).finally(() => {
    restoring.value = false;
  });
  if (!last) return;
  const { request: req } = last;
  Object.assign(form, {
    table: req.table,
    module: req.module,
    resource: req.resource,
    name: req.name,
    moduleTitle: req.moduleTitle
  });
  await loadFields(req.table);
  const overrides = new Map<string, CodegenFieldOption>(
    (req.fields ?? []).map(f => [f.column, f])
  );
  fields.value = fields.value.map(f => {
    const o = overrides.get(f.column);
    return o ? { ...f, ...o } : f;
  });
  advanced.value = {
    keyMode: req.keyMode ?? "",
    readOnly: req.readOnly ?? false,
    refs: (req.fields ?? [])
      .filter(f => f.refTable)
      .map(f => ({
        column: f.column,
        refTable: f.refTable ?? "",
        refKey: f.refKey ?? "",
        relation: f.relation ?? ""
      })),
    children: req.children ?? []
  };
  result.value = last.result;
}
const { copy, isSupported } = useClipboard({ legacy: true });

const pattern = (re: RegExp, msg: string) => ({
  validator: (_rule: unknown, value: string, cb: (e?: Error) => void) =>
    !value || re.test(value) ? cb() : cb(new Error(msg)),
  trigger: "blur"
});
const noUnsafe = {
  validator: (_rule: unknown, value: string, cb: (e?: Error) => void) =>
    UNSAFE_RE.test(value ?? "")
      ? cb(new Error("不能包含引号、反斜杠、尖括号或花括号"))
      : cb(),
  trigger: "blur"
};

const rules: FormRules = {
  table: [{ required: true, message: "请选择数据表", trigger: "change" }],
  module: [
    { required: true, message: "请输入模块名", trigger: "blur" },
    pattern(MODULE_RE, "小写字母开头的 2–31 位小写字母或数字")
  ],
  resource: [
    { required: true, message: "请输入资源名", trigger: "blur" },
    pattern(RESOURCE_RE, "kebab-case，如 customer、sales-order")
  ],
  name: [
    { required: true, message: "请输入中文名", trigger: "blur" },
    { max: 20, message: "最长 20 个字符", trigger: "blur" },
    noUnsafe
  ],
  moduleTitle: [
    { max: 20, message: "最长 20 个字符", trigger: "blur" },
    noUnsafe
  ]
};

/** 与默认值不同的字段配置（保持命令行参数简短） */
function fieldOptions(): CodegenFieldOption[] {
  const byColumn = new Map(defaults.value.map(f => [f.column, f]));
  const out: CodegenFieldOption[] = [];
  for (const f of fields.value) {
    const d = byColumn.get(f.column);
    if (!d) continue;
    const o: CodegenFieldOption = { column: f.column };
    if (f.label.trim() !== d.label) o.label = f.label.trim();
    if (f.list !== d.list) o.list = f.list;
    if (f.form !== d.form) o.form = f.form;
    if (f.search !== d.search) o.search = f.search;
    if ((f.dict ?? "") !== d.dict) o.dict = f.dict ?? "";
    if ((f.upload ?? "") !== d.upload) o.upload = f.upload ?? "";
    if ((f.relation ?? "") !== d.relation) o.relation = f.relation ?? "";
    if (Object.keys(o).length > 1) out.push(o);
  }
  return out;
}

const fieldsValid = computed(() =>
  fields.value.every(f => {
    const label = f.label.trim();
    return label && label.length <= 20 && !UNSAFE_RE.test(label);
  })
);

/** 字段配置加上手动声明的关联（同一列合并为一项） */
function allFieldOptions(): CodegenFieldOption[] {
  const out = fieldOptions();
  for (const r of advanced.value.refs) {
    if (!r.column || !r.refTable) continue;
    let o = out.find(x => x.column === r.column);
    if (!o) {
      o = { column: r.column };
      out.push(o);
    }
    o.refTable = r.refTable.trim();
    if (r.refKey.trim()) o.refKey = r.refKey.trim();
    if (r.relation.trim()) o.relation = r.relation.trim();
  }
  return out;
}

function request(): CodegenRequest {
  const a = advanced.value;
  const children = a.readOnly ? [] : a.children.filter(c => c.table);
  return {
    ...form,
    name: form.name.trim(),
    fields: allFieldOptions(),
    ...(a.keyMode ? { keyMode: a.keyMode } : {}),
    ...(a.readOnly ? { readOnly: true } : {}),
    ...(children.length ? { children } : {})
  };
}

const command = computed(() => {
  if (preview.value) return preview.value.command;
  const parts = [
    "cd server && go run ./cmd/gen",
    `-table ${form.table || "<table>"}`,
    `-module ${form.module || "<module>"}`,
    `-resource ${form.resource || "<resource>"}`,
    `-name '${form.name.trim() || "<中文名>"}'`
  ];
  if (form.moduleTitle) parts.push(`-module-title '${form.moduleTitle}'`);
  const req = request();
  if (req.fields?.length) parts.push(`-fields '${JSON.stringify(req.fields)}'`);
  if (req.keyMode) parts.push(`-key-mode ${req.keyMode}`);
  if (req.readOnly) parts.push("-read-only");
  if (req.children?.length) {
    parts.push(`-children '${JSON.stringify(req.children)}'`);
  }
  return parts.join(" ");
});

/** 由表名推断资源名：去掉 biz_ 之类的前缀，转 kebab-case，去掉复数 s */
function guessResource(table: string): string {
  const base = table.replace(/^[a-z]+_(?=[a-z])/, "").replace(/_/g, "-");
  if (base.endsWith("ies")) return `${base.slice(0, -3)}y`;
  if (/(ses|xes|ches|shes)$/.test(base)) return base.slice(0, -2);
  return base.endsWith("s") ? base.slice(0, -1) : base;
}

function onTableChange(name: string): void {
  const t = tables.value.find(x => x.name === name);
  if (!t) return;
  form.resource = guessResource(t.name);
  if (t.comment) form.name = t.comment.slice(0, 20);
  advanced.value = emptyAdvanced();
  preview.value = null;
  result.value = null;
  loadFields(name);
}

async function loadFields(table: string): Promise<void> {
  loadingFields.value = true;
  try {
    defaults.value = (await getCodegenFields(table)) ?? [];
  } catch {
    defaults.value = [];
  } finally {
    loadingFields.value = false;
  }
  resetFields();
}

function resetFields(): void {
  fields.value = defaults.value.map(f => ({ ...f }));
}

async function loadTables(): Promise<void> {
  loadingTables.value = true;
  loadError.value = "";
  try {
    tables.value = (await listCodegenTables()) ?? [];
  } catch (err) {
    tables.value = [];
    if (isApiError(err) && err.code === CODEGEN_DISABLED) {
      disabled.value = true;
    } else {
      loadError.value = isApiError(err) ? err.message : "加载数据表失败";
    }
  } finally {
    loadingTables.value = false;
  }
}

async function validateAll(): Promise<boolean> {
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return false;
  if (!fieldsValid.value) {
    message("请先修正字段配置中的中文名", { type: "warning" });
    return false;
  }
  return true;
}

async function onPreview(): Promise<void> {
  if (!(await validateAll())) return;
  clearFieldErrors();
  previewing.value = true;
  try {
    preview.value = await previewCodegen(request());
  } catch (err) {
    preview.value = null;
    applyFieldErrors(err);
  } finally {
    previewing.value = false;
  }
}

async function onGenerate(): Promise<void> {
  if (!(await validateAll())) return;
  try {
    await ElMessageBox.confirm(
      overwrite.value
        ? "将把生成的代码写入项目，并覆盖已存在的同名文件。确定继续吗？"
        : "将把生成的代码写入项目（已存在的文件不会被覆盖），新模块会自动接入。确定继续吗？",
      "生成到项目",
      { type: "warning", confirmButtonText: "生成", cancelButtonText: "取消" }
    );
  } catch {
    return;
  }
  clearFieldErrors();
  generating.value = true;
  markPending(true);
  try {
    result.value = await generateCodegen({
      ...request(),
      force: overwrite.value
    });
    // 新页面文件会让开发服务器稍后刷新整个页面，标记保留片刻以便刷新后恢复
    setTimeout(() => markPending(false), 10_000);
    message("代码已生成", { type: "success" });
    preview.value = await previewCodegen(request()).catch(() => preview.value);
  } catch (err) {
    markPending(false);
    result.value = null;
    if (isApiError(err) && err.code === CODEGEN_EXISTS) {
      message(err.message, { type: "warning", duration: 6000 });
    } else if (isApiError(err)) {
      applyFieldErrors(err);
      message(err.message, { type: "error" });
    } else {
      message("生成失败", { type: "error" });
    }
  } finally {
    generating.value = false;
  }
}

async function onCopyCommand(): Promise<void> {
  try {
    await copy(command.value);
    message("命令已复制", { type: "success" });
  } catch {
    message("复制失败，请手动复制", { type: "error" });
  }
}

async function loadDicts(): Promise<void> {
  dicts.value = (await listCodegenDicts().catch(() => [])) ?? [];
}

onMounted(async () => {
  loadDicts();
  await loadTables();
  await restoreLast();
});
</script>

<template>
  <div class="main">
    <el-card v-if="disabled" shadow="never">
      <el-empty description="代码生成仅在开发环境可用" :image-size="120">
        <div
          class="mx-auto max-w-[640px] text-left text-sm leading-6 text-[var(--el-text-color-regular)]"
        >
          <p>
            当前服务不是开发环境（app.env ≠
            dev），为避免暴露数据库结构，读取表结构和预览代码的接口已关闭。
          </p>
          <p class="mb-3">
            请在本地开发环境中打开本页，或在仓库中直接运行命令行工具：
          </p>
          <CodeBlock
            :code="CLI_USAGE"
            language="typescript"
            title="命令行"
            :line-numbers="false"
          />
        </div>
      </el-empty>
    </el-card>

    <template v-else>
      <el-card
        v-loading="restoring"
        shadow="never"
        class="mb-3"
        element-loading-text="正在恢复刚才的生成结果…"
      >
        <template #header>
          <div class="flex items-center justify-between">
            <span class="font-medium">代码生成</span>
            <span class="text-xs text-[var(--el-text-color-secondary)]">
              「生成预览」不写入文件；「生成到项目」写入文件并自动接入新模块（仅开发环境）
            </span>
          </div>
        </template>
        <el-alert
          v-if="loadError"
          type="error"
          :title="loadError"
          :closable="false"
          show-icon
          class="mb-3"
        />
        <el-form
          ref="formRef"
          :model="form"
          :rules="rules"
          :validate-on-rule-change="false"
          label-width="90px"
          class="codegen-form"
        >
          <el-form-item label="数据表" prop="table" :error="fieldErrors.table">
            <el-select
              v-model="form.table"
              filterable
              placeholder="选择数据表"
              :loading="loadingTables"
              no-data-text="当前库中没有可用的业务表（sys_* 已排除）"
              class="w-full"
              @change="onTableChange"
            >
              <el-option
                v-for="t in tables"
                :key="t.name"
                :label="t.name"
                :value="t.name"
                :disabled="!t.supported"
              >
                <div class="flex items-center justify-between gap-3">
                  <span class="font-mono">{{ t.name }}</span>
                  <span
                    class="truncate text-xs text-[var(--el-text-color-secondary)]"
                  >
                    {{
                      t.supported ? t.comment || `${t.columns} 列` : t.reason
                    }}
                  </span>
                </div>
              </el-option>
            </el-select>
          </el-form-item>
          <el-form-item
            label="模块名"
            prop="module"
            :error="fieldErrors.module"
          >
            <el-input v-model="form.module" placeholder="如 sales" clearable />
          </el-form-item>
          <el-form-item
            label="资源名"
            prop="resource"
            :error="fieldErrors.resource"
          >
            <el-input
              v-model="form.resource"
              placeholder="如 order"
              clearable
            />
          </el-form-item>
          <el-form-item label="中文名" prop="name" :error="fieldErrors.name">
            <el-input
              v-model="form.name"
              placeholder="如 订单"
              maxlength="20"
              clearable
            />
          </el-form-item>
          <el-form-item
            label="模块标题"
            prop="moduleTitle"
            :error="fieldErrors.moduleTitle"
          >
            <el-input
              v-model="form.moduleTitle"
              placeholder="新模块的目录菜单标题，如 销售管理（可选）"
              maxlength="20"
              clearable
            />
          </el-form-item>
        </el-form>
        <FieldConfig
          v-if="form.table"
          v-model="fields"
          :loading="loadingFields"
          :dicts="dicts"
          class="mb-4"
          @reset="resetFields"
        />
        <AdvancedOptions
          v-if="form.table"
          v-model="advanced"
          :columns="defaults.map(f => f.column)"
          :tables="tables"
          :dicts="dicts"
          class="mb-4"
        />
        <div class="mb-4 flex flex-wrap items-center gap-2">
          <el-button :loading="previewing" @click="onPreview">
            生成预览
          </el-button>
          <el-button type="primary" :loading="generating" @click="onGenerate">
            生成到项目
          </el-button>
          <el-checkbox v-model="overwrite" class="ml-1">
            覆盖已有文件
          </el-checkbox>
          <el-button link :loading="loadingTables" @click="loadTables">
            刷新数据表
          </el-button>
        </div>
        <el-alert
          v-if="result"
          type="success"
          :closable="false"
          show-icon
          class="mb-4"
          title="代码已写入项目"
        >
          <div class="text-[13px] leading-6">
            <div>
              已写入 {{ result.files.length }} 个文件{{ wiredText }}。接下来：
            </div>
            <ol class="ml-4 list-decimal">
              <li>
                重启后端（make run），启动时会自动注册新模块的菜单和权限；
              </li>
              <li>
                在「角色管理」中给需要的角色分配新权限（超级管理员自动拥有）；
              </li>
              <li>刷新浏览器，左侧菜单即出现新页面。</li>
            </ol>
            <div
              v-for="n in result.notes"
              :key="n"
              class="text-[var(--el-text-color-secondary)]"
            >
              · {{ n }}
            </div>
          </div>
        </el-alert>
        <div class="flex items-start gap-2">
          <CodeBlock
            :code="command"
            language="typescript"
            title="CLI 命令"
            :line-numbers="false"
            class="min-w-0 flex-1"
          />
          <el-button v-if="isSupported" class="mt-1" @click="onCopyCommand">
            复制命令
          </el-button>
        </div>
      </el-card>

      <el-card v-loading="previewing" shadow="never">
        <PreviewPanel v-if="preview" :preview="preview" />
        <el-empty
          v-else
          description="选择数据表并填写模块信息后点击「生成预览」"
          :image-size="100"
        />
      </el-card>
    </template>
  </div>
</template>

<style scoped>
.codegen-form {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  column-gap: 16px;
}
</style>
