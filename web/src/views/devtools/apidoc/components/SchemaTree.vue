<script setup lang="ts">
import { computed } from "vue";
import type { SchemaField } from "@/api/devtools/apidoc";

defineOptions({ name: "DevtoolsSchemaTree" });

const props = defineProps<{ fields: SchemaField[] }>();

type Row = SchemaField & { key: string; rows?: Row[] };

function toRows(fields: SchemaField[], prefix: string): Row[] {
  return fields.map(f => {
    const key = `${prefix}/${f.name}`;
    const children = f.children?.length ? toRows(f.children, key) : undefined;
    return { ...f, key, rows: children };
  });
}

const rows = computed(() => toRows(props.fields, ""));
</script>

<template>
  <el-table
    :data="rows"
    row-key="key"
    :tree-props="{ children: 'rows' }"
    default-expand-all
    border
    size="small"
    class="schema-tree"
  >
    <el-table-column label="字段" min-width="200">
      <template #default="{ row }">
        <span class="font-mono">{{ row.name }}</span>
        <el-tooltip
          v-if="row.circular"
          content="引用了外层同一结构，不再展开"
          placement="top"
        >
          <el-tag size="small" type="info" class="ml-1">循环引用</el-tag>
        </el-tooltip>
      </template>
    </el-table-column>
    <el-table-column label="类型" min-width="180">
      <template #default="{ row }">
        <span class="font-mono text-[var(--el-color-primary)]">
          {{ row.type }}
        </span>
        <el-tag
          v-if="row.nullable"
          size="small"
          type="info"
          effect="plain"
          class="ml-1"
        >
          可为 null
        </el-tag>
      </template>
    </el-table-column>
    <el-table-column label="必填" width="70" align="center">
      <template #default="{ row }">
        <el-tag v-if="row.required" size="small" type="danger" effect="plain">
          是
        </el-tag>
        <span v-else class="text-[var(--el-text-color-placeholder)]">-</span>
      </template>
    </el-table-column>
    <el-table-column label="约束" min-width="160">
      <template #default="{ row }">
        <el-tag
          v-for="c in row.constraints"
          :key="c"
          size="small"
          effect="plain"
          class="mr-1 mb-0.5"
        >
          {{ c }}
        </el-tag>
        <span
          v-if="row.constraints.length === 0"
          class="text-[var(--el-text-color-placeholder)]"
        >
          -
        </span>
      </template>
    </el-table-column>
    <el-table-column label="说明" min-width="160" show-overflow-tooltip>
      <template #default="{ row }">{{ row.description || "-" }}</template>
    </el-table-column>
  </el-table>
</template>
