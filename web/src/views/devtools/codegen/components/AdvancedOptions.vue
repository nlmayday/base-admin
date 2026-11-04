<script setup lang="ts">
import type {
  CodegenDict,
  CodegenKeyMode,
  CodegenTable
} from "@/api/devtools/codegen";
import type { AdvancedModel } from "./advanced";

defineOptions({ name: "CodegenAdvancedOptions" });

defineProps<{
  /** 当前表的列，用于选择关联列 */
  columns: string[];
  tables: CodegenTable[];
  dicts: CodegenDict[];
}>();
const model = defineModel<AdvancedModel>({ required: true });

const KEY_MODES: { value: CodegenKeyMode; label: string }[] = [
  { value: "auto", label: "数据库自增" },
  { value: "generated", label: "程序生成（雪花 ID）" },
  { value: "input", label: "新增时填写" }
];

function addRef(): void {
  model.value.refs.push({ column: "", refTable: "", refKey: "", relation: "" });
}

function addChild(): void {
  model.value.children.push({
    table: "",
    foreignKey: "",
    keyColumn: "lang",
    keySource: "dict",
    keyDict: "",
    keyTable: "",
    keyValueColumn: "code",
    keyLabelColumn: "name",
    keyFilter: "",
    keySortColumn: "",
    keyRequiredColumn: "",
    label: "多语言"
  });
}
</script>

<template>
  <div class="advanced">
    <div class="mb-2 text-sm font-medium">高级选项</div>
    <el-form label-width="90px" size="small">
      <el-form-item label="主键来源">
        <el-select
          v-model="model.keyMode"
          placeholder="自动（按主键列判断）"
          clearable
          class="w-[240px]!"
          @clear="model.keyMode = ''"
        >
          <el-option
            v-for="o in KEY_MODES"
            :key="o.value"
            :label="o.label"
            :value="o.value"
          />
        </el-select>
        <span class="hint">
          bigint 主键不是自增时默认程序生成，文本、整数主键默认新增时填写
        </span>
      </el-form-item>
      <el-form-item label="只读">
        <el-switch v-model="model.readOnly" />
        <span class="hint">
          只生成列表、详情和导出，不生成表单和写接口（流水、订单、日志）
        </span>
      </el-form-item>
      <el-form-item label="软删除">
        <el-switch v-model="model.softDelete" />
        <span class="hint">
          表有 deleted_at 时：开启则删除只做标记、列表隐藏已删除行；关闭则
          deleted_at 是普通时间字段（如注销时间），列出全部行，删除即删除行
        </span>
      </el-form-item>
      <el-form-item label="手动关联">
        <div class="w-full">
          <div
            v-for="(r, i) in model.refs"
            :key="i"
            class="mb-1 flex flex-wrap items-center gap-1"
          >
            <el-select
              v-model="r.column"
              placeholder="本表列（bigint）"
              filterable
              class="w-[150px]!"
            >
              <el-option v-for="c in columns" :key="c" :label="c" :value="c" />
            </el-select>
            <span>→</span>
            <el-select
              v-model="r.refTable"
              placeholder="关联表"
              filterable
              allow-create
              class="w-[160px]!"
            >
              <el-option
                v-for="t in tables"
                :key="t.name"
                :label="t.name"
                :value="t.name"
              />
            </el-select>
            <el-input
              v-model="r.refKey"
              placeholder="关联列（默认 id）"
              class="w-[130px]!"
            />
            <el-input
              v-model="r.relation"
              placeholder="显示列，如 nickname"
              class="w-[150px]!"
            />
            <el-button link type="danger" @click="model.refs.splice(i, 1)">
              删除
            </el-button>
          </div>
          <el-button link type="primary" @click="addRef">添加关联</el-button>
          <div class="hint ml-0!">
            没有外键约束时使用，如 user_id →
            lg_user.nickname；列表显示名称，表单为可搜索的下拉
          </div>
        </div>
      </el-form-item>
      <el-form-item v-if="!model.readOnly" label="子表">
        <div class="w-full">
          <div
            v-for="(c, i) in model.children"
            :key="i"
            class="mb-1 flex flex-wrap items-center gap-1"
          >
            <el-select
              v-model="c.table"
              placeholder="子表，如 xxx_i18n"
              filterable
              allow-create
              class="w-[170px]!"
            >
              <el-option
                v-for="t in tables"
                :key="t.name"
                :label="t.name"
                :value="t.name"
              />
            </el-select>
            <el-input
              v-model="c.foreignKey"
              placeholder="外键列，如 item_id"
              class="w-[140px]!"
            />
            <el-input
              v-model="c.keyColumn"
              placeholder="键列，如 lang"
              class="w-[110px]!"
            />
            <el-select v-model="c.keySource" class="w-[90px]!">
              <el-option label="键来自字典" value="dict" />
              <el-option label="键来自表" value="table" />
            </el-select>
            <el-select
              v-if="c.keySource === 'dict'"
              v-model="c.keyDict"
              placeholder="键字典"
              filterable
              class="w-[140px]!"
            >
              <el-option
                v-for="d in dicts"
                :key="d.code"
                :label="`${d.name}（${d.code}）`"
                :value="d.code"
              />
            </el-select>
            <template v-else>
              <el-select
                v-model="c.keyTable"
                placeholder="键表，如 lg_language"
                filterable
                allow-create
                class="w-[160px]!"
              >
                <el-option
                  v-for="t in tables"
                  :key="t.name"
                  :label="t.name"
                  :value="t.name"
                />
              </el-select>
              <el-input
                v-model="c.keyValueColumn"
                placeholder="值列 code"
                class="w-[90px]!"
              />
              <el-input
                v-model="c.keyLabelColumn"
                placeholder="名称列 name"
                class="w-[100px]!"
              />
              <el-input
                v-model="c.keyFilter"
                placeholder="条件，如 enabled = 1"
                class="w-[150px]!"
              />
              <el-input
                v-model="c.keySortColumn"
                placeholder="排序列 sort"
                class="w-[100px]!"
              />
              <el-input
                v-model="c.keyRequiredColumn"
                placeholder="必填列 required"
                class="w-[110px]!"
              />
            </template>
            <el-input
              v-model="c.label"
              placeholder="标题"
              maxlength="20"
              class="w-[100px]!"
            />
            <el-button link type="danger" @click="model.children.splice(i, 1)">
              删除
            </el-button>
          </div>
          <el-button link type="primary" @click="addChild">添加子表</el-button>
          <div class="hint ml-0!">
            子表随主表一起编辑：表单按键（字典的每一项，或键表中符合条件的每一行，如每种语言）列出一组输入，全部为空的不保存；键表有必填列时，必填的键必须填写
          </div>
        </div>
      </el-form-item>
    </el-form>
  </div>
</template>

<style scoped>
.advanced {
  padding: 12px 16px 4px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 4px;
}

.hint {
  margin-left: 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
