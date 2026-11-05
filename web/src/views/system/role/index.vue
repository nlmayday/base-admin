<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { usePagedTable } from "@/composables/usePagedTable";
import { confirmAction } from "@/composables/useConfirmAction";
import { formatDateTime } from "@/utils/format";
import { getDeptTree, type Dept } from "@/api/iam/dept";
import {
  DATA_SCOPE_OPTIONS,
  deleteRole,
  listRoles,
  type Role
} from "@/api/iam/role";
import ReStatusTag from "@/components/ReStatusTag/index.vue";
import RoleFormDialog from "./components/RoleFormDialog.vue";
import RolePermDrawer from "./components/RolePermDrawer.vue";
import { hasPerm } from "@/utils/permission";
import { computed } from "vue";

/** 没有任何行操作权限时隐藏「操作」列 */
const showRowActions = computed(() =>
  ["iam:role:update", "iam:role:grant", "iam:role:delete"].some(code =>
    hasPerm(code)
  )
);

defineOptions({ name: "IamRole" });

const filter = reactive({ keyword: "" });
const {
  loading,
  rows,
  pagination,
  reload,
  search,
  onSizeChange,
  onCurrentChange
} = usePagedTable<Role>(query => listRoles({ ...query, ...filter }));

const deptTree = ref<Dept[]>([]);
const dialogVisible = ref(false);
const drawerVisible = ref(false);
const editing = ref<Role | null>(null);
const granting = ref<Role | null>(null);

const scopeLabel = (scope: string): string =>
  DATA_SCOPE_OPTIONS.find(o => o.value === scope)?.label ?? scope;

async function loadDeptTree(): Promise<void> {
  try {
    // 部门树需要 iam:dept:view 或 iam:user:view，没有时静默降级（自定义数据范围无法选部门）
    deptTree.value = (await getDeptTree({ skipErrorHandler: true })) ?? [];
  } catch {
    deptTree.value = [];
  }
}

function openCreate(): void {
  editing.value = null;
  dialogVisible.value = true;
}

function openEdit(row: Role): void {
  editing.value = row;
  dialogVisible.value = true;
}

function openGrant(row: Role): void {
  granting.value = row;
  drawerVisible.value = true;
}

function remove(row: Role): void {
  confirmAction({
    confirm: `确定删除角色「${row.name}」吗？`,
    success: "已删除",
    action: () => deleteRole(row.id),
    reload
  });
}

function resetFilter(): void {
  filter.keyword = "";
  search();
}

onMounted(() => {
  loadDeptTree();
  reload();
});
</script>

<template>
  <div class="main">
    <el-card shadow="never" class="mb-3" body-class="pb-0!">
      <el-form :inline="true" :model="filter" @submit.prevent="search">
        <el-form-item label="关键字">
          <el-input
            v-model="filter.keyword"
            placeholder="角色编码/名称"
            clearable
            class="w-[220px]!"
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" @click="search">
            搜索
          </el-button>
          <el-button @click="resetFilter">重置</el-button>
        </el-form-item>
      </el-form>
    </el-card>
    <el-card shadow="never">
      <div class="mb-3 flex justify-between">
        <span class="card-title">角色列表</span>
        <el-button
          v-perm="'iam:role:create'"
          type="primary"
          data-testid="role-create"
          @click="openCreate"
        >
          新增角色
        </el-button>
      </div>
      <el-table v-loading="loading" :data="rows" row-key="id" border>
        <el-table-column prop="code" label="编码" min-width="140">
          <template #default="{ row }">
            {{ row.code }}
            <el-tag v-if="row.builtin" size="small" type="warning" class="ml-1">
              内置
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column label="数据范围" min-width="120">
          <template #default="{ row }">{{
            scopeLabel(row.dataScope)
          }}</template>
        </el-table-column>
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <ReStatusTag :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column prop="sort" label="排序" width="70" align="center" />
        <el-table-column prop="remark" label="备注" min-width="140" />
        <el-table-column label="创建时间" width="215">
          <template #default="{ row }">{{
            formatDateTime(row.createdAt)
          }}</template>
        </el-table-column>
        <el-table-column
          v-if="showRowActions"
          label="操作"
          width="220"
          fixed="right"
        >
          <template #default="{ row }">
            <el-button
              v-perm="'iam:role:update'"
              link
              type="primary"
              :disabled="row.builtin"
              @click="openEdit(row as Role)"
            >
              编辑
            </el-button>
            <el-button
              v-perm="'iam:role:grant'"
              link
              type="primary"
              :disabled="row.builtin"
              @click="openGrant(row as Role)"
            >
              分配权限
            </el-button>
            <el-button
              v-perm="'iam:role:delete'"
              link
              type="danger"
              :disabled="row.builtin"
              @click="remove(row as Role)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        class="mt-3 justify-end"
        :current-page="pagination.currentPage"
        :page-size="pagination.pageSize"
        :page-sizes="[10, 20, 50, 100]"
        :total="pagination.total"
        layout="total, sizes, prev, pager, next, jumper"
        background
        @size-change="onSizeChange"
        @current-change="onCurrentChange"
      />
    </el-card>

    <RoleFormDialog
      v-model="dialogVisible"
      :role="editing"
      :dept-tree="deptTree"
      @saved="reload"
      @reload="reload"
    />
    <RolePermDrawer v-model="drawerVisible" :role="granting" />
  </div>
</template>
