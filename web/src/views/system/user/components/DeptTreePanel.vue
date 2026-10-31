<script setup lang="ts">
import { ref, watch } from "vue";
import type { ElTree } from "element-plus";
import type { Dept } from "@/api/iam/dept";

defineOptions({ name: "DeptTreePanel" });

defineProps<{ data: Dept[]; loading: boolean }>();
const emit = defineEmits<{ (e: "select", deptId: string | null): void }>();

const treeRef = ref<InstanceType<typeof ElTree>>();
const keyword = ref("");
const selectedId = ref<string | null>(null);

watch(keyword, value => treeRef.value?.filter(value));

function filterNode(value: string, data: Dept): boolean {
  return !value || data.name.includes(value);
}

function onNodeClick(node: Dept): void {
  // 再次点击同一部门取消筛选
  selectedId.value = selectedId.value === node.id ? null : node.id;
  if (selectedId.value === null) treeRef.value?.setCurrentKey(undefined);
  emit("select", selectedId.value);
}

function clear(): void {
  selectedId.value = null;
  treeRef.value?.setCurrentKey(undefined);
  emit("select", null);
}

defineExpose({ clear });
</script>

<template>
  <el-card shadow="never" class="h-full" body-class="p-3!">
    <template #header>
      <div class="flex items-center justify-between">
        <span class="font-medium">部门</span>
        <el-button link type="primary" @click="clear">全部</el-button>
      </div>
    </template>
    <el-input
      v-model="keyword"
      placeholder="搜索部门"
      clearable
      size="small"
      class="mb-2"
    />
    <el-tree
      ref="treeRef"
      v-loading="loading"
      :data="data"
      node-key="id"
      :props="{ label: 'name', children: 'children' }"
      :filter-node-method="filterNode"
      :expand-on-click-node="false"
      highlight-current
      default-expand-all
      @node-click="onNodeClick"
    />
  </el-card>
</template>
