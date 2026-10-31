<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from "vue";
import { confirmAction } from "@/composables/useConfirmAction";
import { deleteDept, getDeptTree, type Dept } from "@/api/iam/dept";
import ReStatusTag from "@/components/ReStatusTag/index.vue";
import DeptFormDialog from "./components/DeptFormDialog.vue";
import { hasPerm } from "@/utils/permission";

/** 没有任何行操作权限时隐藏「操作」列 */
const showRowActions = computed(() =>
  ["iam:dept:create", "iam:dept:update", "iam:dept:delete"].some(code =>
    hasPerm(code)
  )
);

defineOptions({ name: "IamDept" });

const loading = ref(false);
const tree = ref<Dept[]>([]);
const keyword = ref("");
const expandAll = ref(true);
const tableKey = ref(0);
const dialogVisible = ref(false);
const editing = ref<Dept | null>(null);
const presetParentId = ref<string | null>(null);

/** 按名称过滤：保留命中节点及其祖先 */
function filterTree(nodes: Dept[], kw: string): Dept[] {
  if (!kw) return nodes;
  return nodes.flatMap(node => {
    const children = filterTree(node.children ?? [], kw);
    return node.name.includes(kw) || children.length
      ? [{ ...node, children }]
      : [];
  });
}

const visibleTree = computed(() =>
  filterTree(tree.value, keyword.value.trim())
);

async function reload(): Promise<void> {
  loading.value = true;
  try {
    tree.value = (await getDeptTree()) ?? [];
  } catch {
    tree.value = [];
  } finally {
    loading.value = false;
  }
}

async function toggleExpand(): Promise<void> {
  expandAll.value = !expandAll.value;
  // 重新渲染表格以应用 default-expand-all
  tableKey.value += 1;
  await nextTick();
}

function openCreate(parent: Dept | null): void {
  editing.value = null;
  presetParentId.value = parent?.id ?? null;
  dialogVisible.value = true;
}

function openEdit(row: Dept): void {
  editing.value = row;
  presetParentId.value = null;
  dialogVisible.value = true;
}

function remove(row: Dept): void {
  confirmAction({
    confirm: `确定删除部门「${row.name}」吗？`,
    success: "已删除",
    action: () => deleteDept(row.id),
    reload
  });
}

onMounted(reload);
</script>

<template>
  <div class="main">
    <el-card shadow="never">
      <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-2">
          <el-input
            v-model="keyword"
            placeholder="搜索部门名称"
            clearable
            class="w-[220px]!"
          />
          <el-button @click="toggleExpand">
            {{ expandAll ? "全部折叠" : "全部展开" }}
          </el-button>
          <el-button :loading="loading" @click="reload">刷新</el-button>
        </div>
        <el-button
          v-perm="'iam:dept:create'"
          type="primary"
          data-testid="dept-create"
          @click="openCreate(null)"
        >
          新增部门
        </el-button>
      </div>
      <el-table
        :key="tableKey"
        v-loading="loading"
        :data="visibleTree"
        row-key="id"
        border
        :default-expand-all="expandAll"
        :tree-props="{ children: 'children' }"
      >
        <el-table-column prop="name" label="部门名称" min-width="200" />
        <el-table-column prop="sort" label="排序" width="80" align="center" />
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <ReStatusTag :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column
          v-if="showRowActions"
          label="操作"
          width="220"
          fixed="right"
        >
          <template #default="{ row }">
            <el-button
              v-perm="'iam:dept:create'"
              link
              type="primary"
              @click="openCreate(row as Dept)"
            >
              新增下级
            </el-button>
            <el-button
              v-perm="'iam:dept:update'"
              link
              type="primary"
              @click="openEdit(row as Dept)"
            >
              编辑
            </el-button>
            <el-button
              v-perm="'iam:dept:delete'"
              link
              type="danger"
              @click="remove(row as Dept)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <DeptFormDialog
      v-model="dialogVisible"
      :dept="editing"
      :parent-id="presetParentId"
      :tree="tree"
      @saved="reload"
      @reload="reload"
    />
  </div>
</template>
