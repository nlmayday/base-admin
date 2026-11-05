<script setup lang="ts">
import { computed } from "vue";
import type { CodegenFieldInfo, CodegenFormat } from "@/api/devtools/codegen";
import { splitValues } from "./extras";

defineOptions({ name: "CodegenFieldExtras" });

/** 一个字段的显示、表单和搜索细节（字段配置表的展开行） */
const props = defineProps<{ columns: string[] }>();
const f = defineModel<CodegenFieldInfo>({ required: true });

const FORMATS: { value: CodegenFormat; label: string; numeric: boolean }[] = [
  { value: "coins", label: "千分位 1,234", numeric: true },
  { value: "signed", label: "带符号 +1,234（正绿负红）", numeric: true },
  { value: "cents", label: "美分 → $12.34", numeric: true },
  { value: "permille", label: "千分比 → +5.5%", numeric: true },
  { value: "milli", label: "千分之一 → 1.50（可加后缀）", numeric: true },
  { value: "image", label: "图片缩略图", numeric: false }
];
const formats = computed(() =>
  FORMATS.filter(o => o.numeric === f.value.numeric)
);
const others = computed(() => props.columns.filter(c => c !== f.value.column));
const extraLabels = computed(() =>
  f.value.relationLabels.filter(l => l !== f.value.relation)
);

const showWhenColumn = computed({
  get: () => f.value.showWhen?.column ?? "",
  set: column => {
    f.value.showWhen = column
      ? { column, in: f.value.showWhen?.in ?? [] }
      : undefined;
  }
});
const showWhenValues = computed({
  get: () => (f.value.showWhen?.in ?? []).join(", "),
  set: text => {
    if (f.value.showWhen) f.value.showWhen.in = splitValues(text);
  }
});
const feature = computed({
  get: () => (f.value.showIf ?? "").replace(/^feature:/, ""),
  set: v => {
    f.value.showIf = v.trim() ? `feature:${v.trim()}` : "";
  }
});
</script>

<template>
  <el-form label-width="96px" size="small" class="extras">
    <el-form-item label="显示格式">
      <el-select
        v-model="f.format"
        clearable
        placeholder="原样显示"
        class="w-[240px]!"
      >
        <el-option
          v-for="o in formats"
          :key="o.value"
          :label="o.label"
          :value="o.value"
        />
      </el-select>
      <el-input
        v-if="f.format === 'milli'"
        v-model="f.suffix"
        maxlength="8"
        placeholder="后缀，如 x"
        class="ml-2 w-[100px]!"
      />
      <span class="hint">只影响列表，导出仍为原值</span>
    </el-form-item>
    <el-form-item v-if="f.relation && extraLabels.length" label="附加显示列">
      <el-select
        v-model="f.relationExtra"
        clearable
        placeholder="不附加"
        class="w-[200px]!"
      >
        <el-option v-for="l in extraLabels" :key="l" :label="l" :value="l" />
      </el-select>
      <span class="hint"
        >如 short_id 之外再显示 nickname：100001（玩家甲）</span
      >
    </el-form-item>
    <el-form-item
      v-if="f.search === 'eq' || f.search === 'like'"
      label="搜索默认值"
    >
      <el-input
        v-model="f.searchDefault"
        placeholder="如 0"
        class="w-[200px]!"
      />
      <span class="hint">进入页面和重置时的筛选值</span>
    </el-form-item>
    <template v-if="f.form">
      <el-form-item label="新增默认值">
        <el-input v-model="f.default" class="w-[200px]!" />
      </el-form-item>
      <el-form-item v-if="f.numeric" label="数字范围">
        <el-input-number
          v-model="f.min"
          :controls="false"
          placeholder="最小"
          class="w-[90px]!"
        />
        <el-input-number
          v-model="f.max"
          :controls="false"
          placeholder="最大"
          class="ml-1 w-[90px]!"
        />
        <el-input-number
          v-model="f.step"
          :controls="false"
          placeholder="步长"
          class="ml-1 w-[90px]!"
        />
      </el-form-item>
      <el-form-item label="占位提示">
        <el-input v-model="f.placeholder" maxlength="50" class="w-[240px]!" />
      </el-form-item>
      <el-form-item label="条件显示">
        <el-select
          v-model="showWhenColumn"
          clearable
          placeholder="总是显示"
          class="w-[160px]!"
        >
          <el-option v-for="c in others" :key="c" :label="c" :value="c" />
        </el-select>
        <el-input
          v-if="f.showWhen"
          v-model="showWhenValues"
          placeholder="取这些值时显示，逗号分隔"
          class="ml-2 w-[200px]!"
        />
      </el-form-item>
      <el-form-item v-if="f.textRequired" label="允许为空">
        <el-checkbox
          :model-value="f.required === false"
          @update:model-value="v => (f.required = v ? false : undefined)"
        />
        <span class="hint">NOT NULL 的文本可保存空串（保存前去首尾空格）</span>
      </el-form-item>
    </template>
    <el-form-item label="功能开关">
      <el-input
        v-model="feature"
        placeholder="开关名，如 vip"
        class="w-[200px]!"
      />
      <span class="hint">
        开关关闭时列表和表单都不显示，编辑保留原值；开关由模块的
        featureEnabled(ctx, name) 提供
      </span>
    </el-form-item>
  </el-form>
</template>

<style scoped>
.extras {
  padding: 8px 16px 0;
}

.hint {
  margin-left: 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
